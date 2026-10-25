// Package page keyset sayfalama yardımcısıdır: ?limit=&cursor= (cursor opak, base64url).
package page

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
)

const (
	DefaultLimit = 50
	MaxLimit     = 100
)

// Params istekten okunan sayfa parametreleri. AfterID 0 ise ilk sayfa.
type Params struct {
	Limit   int
	AfterID int64
}

type cursor struct {
	ID int64 `json:"id"`
}

// Parse limit ve cursor'ı okur; hatalı alanları details olarak döner.
func Parse(r *http.Request) (Params, map[string]string) {
	p := Params{Limit: DefaultLimit}
	details := map[string]string{}
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > MaxLimit {
			details["limit"] = "1-" + strconv.Itoa(MaxLimit) + " arası tam sayı olmalı"
		} else {
			p.Limit = n
		}
	}
	if v := q.Get("cursor"); v != "" {
		id, ok := decode(v)
		if !ok {
			details["cursor"] = "geçersiz imleç"
		} else {
			p.AfterID = id
		}
	}
	if len(details) > 0 {
		return p, details
	}
	return p, nil
}

func Encode(id int64) string {
	b, _ := json.Marshal(cursor{ID: id})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decode(s string) (int64, bool) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return 0, false
	}
	var c cursor
	if json.Unmarshal(b, &c) != nil || c.ID <= 0 {
		return 0, false
	}
	return c.ID, true
}

// Response liste yanıtıdır. Items nil ise [] yazılır.
type Response[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

// Build sayfa doluysa son öğenin id'sinden sonraki imleci üretir.
func Build[T any](items []T, limit int, id func(T) int64) Response[T] {
	if items == nil {
		items = []T{}
	}
	resp := Response[T]{Items: items}
	if len(items) == limit {
		c := Encode(id(items[len(items)-1]))
		resp.NextCursor = &c
	}
	return resp
}
