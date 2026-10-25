// Package metrics Prometheus metin biçiminde basit sayaçlar sunar.
// Ek bağımlılık yerine elle yazıldı; yalnızca counter ve süre toplamı var.
package metrics

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/platform/logging"
)

type Registry struct {
	mu         sync.Mutex
	counters   map[string]map[string]float64 // ad -> etiket dizgesi -> değer
	help       map[string]string
	collectors []Collector
	types      map[string]string
}

func NewRegistry() *Registry {
	return &Registry{
		counters: map[string]map[string]float64{},
		help:     map[string]string{},
		types:    map[string]string{},
	}
}

// Describe metriğin HELP/TYPE satırlarını tanımlar.
func (r *Registry) Describe(name, typ, help string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.help[name] = help
	r.types[name] = typ
	if r.counters[name] == nil {
		r.counters[name] = map[string]float64{}
	}
}

// Add etiketli metriği v kadar artırır. labels: "k1","v1","k2","v2"...
func (r *Registry) Add(name string, v float64, labels ...string) {
	key := labelString(labels)
	r.mu.Lock()
	defer r.mu.Unlock()
	m := r.counters[name]
	if m == nil {
		m = map[string]float64{}
		r.counters[name] = m
	}
	m[key] += v
}

func labelString(kv []string) string {
	if len(kv) == 0 {
		return ""
	}
	parts := make([]string, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		parts = append(parts, kv[i]+`="`+escape(kv[i+1])+`"`)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func escape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

// Sample scrape anında okunan değerdir; sonraki isteğe saklanmaz.
type Sample struct {
	Name   string
	Value  int64
	Labels []string
}
type Collector func(context.Context) ([]Sample, error)

func (r *Registry) Collect(collector Collector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.collectors = append(r.collectors, collector)
}

// Handler GET /metrics yanıtını üretir.
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.mu.Lock()
		names := make([]string, 0, len(r.counters))
		for n := range r.counters {
			names = append(names, n)
		}
		sort.Strings(names)
		var b strings.Builder
		for _, n := range names {
			if h := r.help[n]; h != "" {
				fmt.Fprintf(&b, "# HELP %s %s\n", n, h)
			}
			if t := r.types[n]; t != "" {
				fmt.Fprintf(&b, "# TYPE %s %s\n", n, t)
			}
			keys := make([]string, 0, len(r.counters[n]))
			for k := range r.counters[n] {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "%s%s %s\n", n, k, strconv.FormatFloat(r.counters[n][k], 'f', -1, 64))
			}
		}
		collectors := append([]Collector(nil), r.collectors...)
		r.mu.Unlock()
		// DB çağrıları sayaç kilidini tutmaz; her scrape güncel bir snapshot alır.
		for _, collect := range collectors {
			ctx, cancel := context.WithTimeout(req.Context(), 250*time.Millisecond)
			samples, err := collect(ctx)
			cancel()
			if err != nil {
				continue
			}
			for _, sample := range samples {
				fmt.Fprintf(&b, "%s%s %d\n", sample.Name, labelString(sample.Labels), sample.Value)
			}
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(b.String()))
	}
}

// HTTPMiddleware istek sayısını ve toplam süreyi chi rota kalıbına göre sayar
// (ham yol yerine kalıp: kardinalite patlamasın).
func (r *Registry) HTTPMiddleware(next http.Handler) http.Handler {
	r.Describe("http_requests_total", "counter", "HTTP istek sayısı")
	r.Describe("http_request_duration_seconds_sum", "counter", "HTTP istek süresi toplamı (saniye)")
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		rec := &logging.StatusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, req)
		if rec.Status == 0 {
			rec.Status = http.StatusOK
		}
		route := "unmatched"
		if rc := chi.RouteContext(req.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		status := strconv.Itoa(rec.Status)
		r.Add("http_requests_total", 1, "method", req.Method, "route", route, "status", status)
		r.Add("http_request_duration_seconds_sum", time.Since(start).Seconds(), "method", req.Method, "route", route)
	})
}
