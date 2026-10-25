package simulation

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/EnesGoker/guvenpolice/backend/internal/audit"
	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/clock"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
	"github.com/EnesGoker/guvenpolice/backend/internal/proration"
)

type policyPlan struct {
	Ordinal    int
	ID         int64
	Product    pricing.Product
	Start, End time.Time
	Count      int
	Plate      string
	Customer   int
	Losses     []time.Time
	CancelAt   *time.Time
	Unpaid     bool
}

func entityRNG(seed int64, ordinal, domain int) *rand.Rand {
	return rand.New(rand.NewSource(int64(uint64(seed) ^ uint64(ordinal+1)*6364136223846793005 ^ uint64(domain)*1442695040888963407)))
}

func hasDeliberateClaims(i int) bool {
	// İlk iki yakın dönem poliçesi aynı atölyede altı bildirim üretir.
	// Her binde bir yakın dönem ve bir tarihsel üçlü, örnek yoğunluğunu sınırlı tutar.
	return i == 0 || i%1000 == 1 || i%1000 == 200
}

func planPolicy(c Config, i int) policyPlan {
	rng := entityRNG(c.Seed, i, 1)
	// Kasko yıllık ~%12 toplamı: normal %10,7 ve seyrek kasıtlı üçlüler.
	product, incidenceBP := pricing.Kasko, 1070
	if i%100 >= 85 {
		product, incidenceBP = pricing.Konut, 400
	} else if i%100 >= 50 {
		product, incidenceBP = pricing.Trafik, 600
	}
	first := c.Anchor.AddDate(-c.Years, 0, 0)
	days := int(proration.DaysBetween(first, c.Anchor.AddDate(0, 0, -15)))
	start := first.AddDate(0, 0, rng.Intn(days+1))
	count := []int{1, 3, 6, 12}[rng.Intn(4)]
	// Küçük N'de de yakın dönem kuyruk ve 30 gün kapısını ölçülebilir tutan örnekler.
	if i%1000 <= 1 {
		start = c.Anchor.AddDate(0, 0, -18)
	}
	unpaid := i%1000 == 2 || i%1000 == 3
	if unpaid {
		start = c.Anchor.AddDate(0, 0, -45)
		count = 3
	}
	p := policyPlan{Ordinal: i, Product: product, Start: start, End: caldate.AddYear(start), Count: count, Unpaid: unpaid}
	if product.IsVehicle() {
		p.Plate = fmt.Sprintf("%02dSIM%04d", i%81+1, i/81)
	}
	exposure := int(proration.DaysBetween(start, minDate(p.End, c.Anchor)))
	if !unpaid && rng.Intn(3650000) < incidenceBP*max(0, exposure) {
		loss := start.AddDate(0, 0, rng.Intn(max(1, exposure)))
		p.Losses = []time.Time{loss}
	}
	if product.IsVehicle() && hasDeliberateClaims(i) {
		p.Losses = []time.Time{start.AddDate(0, 0, 2), start.AddDate(0, 0, 5), start.AddDate(0, 0, 8)}
	}
	if len(p.Losses) == 0 && !unpaid && rng.Intn(100) < 3 {
		at := start.AddDate(0, 0, 30+rng.Intn(240))
		if !at.After(c.Anchor) {
			p.CancelAt = &at
		}
	}
	return p
}

func minDate(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (r *runner) eventContext(ctx context.Context, uid int64, role auth.Role, at time.Time) context.Context {
	if r.realtime {
		return auth.WithPrincipal(ctx, auth.Principal{UserID: uid, Role: role})
	}
	return auth.WithPrincipal(clock.WithTime(ctx, at.Add(12*time.Hour)), auth.Principal{UserID: uid, Role: role})
}

func (r *runner) policies(ctx context.Context) error {
	if err := r.progress(ctx, "POLICIES", 0, r.cfg.Policies); err != nil {
		return err
	}
	r.plans = make([]policyPlan, r.cfg.Policies)
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	failures := make(chan error, 1)
	var processed atomic.Int64
	var progressMu sync.Mutex
	var wg sync.WaitGroup
	for range r.cfg.Workers {
		wg.Go(func() {
			for i := range jobs {
				p := planPolicy(r.cfg, i)
				p.Customer = i % len(r.refs.customers)
				if err := r.issueAndPay(work, &p); err != nil {
					select {
					case failures <- fmt.Errorf("SIM-%06d: %w", i+1, err):
					default:
					}
					cancel()
					return
				}
				r.plans[i] = p
				n := processed.Add(1)
				if n%1000 == 0 || int(n) == r.cfg.Policies {
					progressMu.Lock()
					// Eşzamanlı worker eski ilerlemeyi sonradan yazamaz.
					err := r.progress(work, "POLICIES", int(processed.Load()), r.cfg.Policies)
					progressMu.Unlock()
					if err != nil {
						select {
						case failures <- err:
						default:
						}
						cancel()
						return
					}
				}
			}
		})
	}
feed:
	for i := range r.cfg.Policies {
		select {
		case jobs <- i:
		case <-work.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	select {
	case err := <-failures:
		return err
	default:
		return ctx.Err()
	}
}

func (r *runner) issueAndPay(ctx context.Context, p *policyPlan) error {
	customer := r.refs.customers[p.Customer]
	agent := r.refs.agents[customer.Agent]
	selected, err := pricing.SelectTariff(r.refs.tariffs[p.Product], p.Product, p.Start)
	if err != nil {
		return err
	}
	age, ncd := caldate.AgeOn(customer.Birth, p.Start), p.Ordinal%4
	risk := policy.RiskData{Risk: pricing.Risk{DriverAge: &age, NCDLevel: &ncd, RegionCode: customer.Region, VehicleGroup: []string{"A", "B", "C", "D"}[p.Ordinal%4]}, Plate: p.Plate, BirthDate: caldate.Format(customer.Birth)}
	if !p.Product.IsVehicle() {
		risk.DriverAge = nil
		risk.VehicleGroup = ""
	}
	var codes []string
	for _, c := range pricing.Catalog[p.Product].Coverages {
		if c.Required || p.Ordinal%3 == 0 {
			codes = append(codes, c.Code)
		}
	}
	result, err := pricing.Calculate(selected, pricing.Input{Product: p.Product, Risk: risk.Risk, Coverages: codes})
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(risk)
	if err != nil {
		return err
	}
	var covers []policy.CoverageTerm
	for _, c := range result.Coverages {
		spec, _ := pricing.CoverageSpecFor(p.Product, c.Code)
		covers = append(covers, policy.CoverageTerm{Code: c.Code, LimitKurus: spec.LimitKurus, DeductibleType: spec.DeductibleType, DeductibleValue: spec.DeductibleValue, PremiumKurus: c.PremiumKurus})
	}
	var plate *string
	if p.Product.IsVehicle() {
		plate = &p.Plate
	}
	issueCtx := r.eventContext(ctx, agent.UserID, auth.RoleAgent, p.Start)
	err = pgx.BeginFunc(issueCtx, r.pool, func(tx pgx.Tx) error {
		issued, err := policy.Issue(issueCtx, tx, policy.IssueInput{PolicyNoOverride: fmt.Sprintf("SIM-%06d", p.Ordinal+1), ActorID: agent.UserID, Product: p.Product, CustomerID: customer.ID, AgentID: &agent.ID, TariffVersionID: selected.ID, StartDate: p.Start, EndDate: p.End, Plate: plate, RiskData: encoded, AnnualPremium: result.TotalKurus, AnnualLimitKurus: pricing.Catalog[p.Product].AnnualLimitKurus, InstallmentCount: p.Count, Coverages: covers})
		if err != nil {
			return err
		}
		p.ID = issued.ID
		return audit.Write(issueCtx, tx, audit.Entry{Action: "POLICY_ISSUED", EntityType: "policy", EntityID: p.ID, New: issued})
	})
	if err != nil {
		return err
	}
	if err = r.payDues(ctx, *p); err != nil {
		return err
	}
	if p.CancelAt != nil {
		at := *p.CancelAt
		_, err = policy.NewChangeService(r.pool, func() time.Time {
			if r.realtime {
				return time.Now()
			}
			return at.Add(12 * time.Hour)
		}, r.cfg.Rate).Cancel(r.eventContext(ctx, agent.UserID, auth.RoleAgent, at), p.ID, policy.CancelRequest{EffectiveDate: caldate.Format(at), Reason: "CUSTOMER_REQUEST", Note: "Kurgusal SIM müşteri iptali"})
	}
	return err
}

func dueBalance(v payment.BillingView, at time.Time) int64 {
	var amount int64
	for _, row := range v.Installments {
		if row.Status != "CANCELLED" && row.DueDate <= caldate.Format(at) {
			amount += row.AmountKurus - row.PaidKurus
		}
	}
	for _, c := range v.Interest {
		amount += c.AmountKurus - c.PaidKurus
	}
	return amount
}

func (r *runner) payDues(ctx context.Context, p policyPlan) error {
	rng := entityRNG(r.cfg.Seed, p.Ordinal, 2)
	at := p.Start
	svc := payment.NewService(r.pool, func() time.Time {
		if r.realtime {
			return time.Now()
		}
		return at.Add(12 * time.Hour)
	}, r.cfg.Rate)
	view, err := svc.Billing(r.eventContext(ctx, r.refs.account, auth.RoleAccounting, at), p.ID)
	if err != nil {
		return err
	}
	rows := view.Installments
	for _, row := range rows {
		due, err := caldate.Parse(row.DueDate)
		if err != nil {
			return err
		}
		at = due.AddDate(0, 0, rng.Intn(6))
		if r.realtime {
			at = caldate.Today(time.Now())
		}
		cutoff := r.cfg.Anchor
		if p.CancelAt != nil {
			cutoff = *p.CancelAt
		}
		if at.After(cutoff) {
			break
		}
		recent := !due.Before(r.cfg.Anchor.AddDate(0, 0, -60))
		if !r.realtime && (p.Unpaid || (recent && rng.Intn(100) < 5)) {
			break
		} // Sonraki ödeme en eski kaçırılan vadeyi kapatmasın.
		payCtx := r.eventContext(ctx, r.refs.account, auth.RoleAccounting, at)
		amount := dueBalance(view, at)
		if amount <= 0 {
			continue
		}
		if _, err = svc.Pay(payCtx, p.ID, payment.PayRequest{AmountKurus: &amount, Method: "TRANSFER", PaymentDate: caldate.Format(at)}); err != nil {
			return err
		}
		view, err = svc.Billing(payCtx, p.ID)
		if err != nil {
			return err
		}
		// Pay yeni faizi tahakkuk ettirir; aynı olay günündeki kalanını servisten okuyarak tamamla.
		remaining := dueBalance(view, at)
		if remaining > 0 {
			if _, err = svc.Pay(payCtx, p.ID, payment.PayRequest{AmountKurus: &remaining, Method: "TRANSFER", PaymentDate: caldate.Format(at)}); err != nil {
				return err
			}
			view, err = svc.Billing(payCtx, p.ID)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
