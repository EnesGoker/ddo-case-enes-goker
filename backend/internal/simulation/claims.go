package simulation

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/claim"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

type claimPlan struct {
	Policy   int
	Index    int
	Loss     time.Time
	Claim    claim.Claim
	Reject   bool
	Hold     bool
	Amount   int64
	Workshop int
}

type claimEvent struct {
	At    time.Time
	Plan  int
	Stage int
}

func (c Config) openSince() time.Time {
	return c.Anchor.Add(24 * time.Hour).Add(-c.OpenWindow)
}

// Olay tabanı eventContext'te öğlene taşınır. Eski dosyalar pencere başlamadan
// tamamlanır; yalnız yeni dosyaların sonraki adımları anchor sonrasına kalabilir.
func claimTimes(c Config, first time.Time, rng *rand.Rand) [8]time.Time {
	times := [8]time.Time{first}
	for stage := 1; stage < len(times); stage++ {
		times[stage] = times[stage-1].AddDate(0, 0, 1+rng.Intn(6))
	}
	cutoff := c.openSince().Add(-12 * time.Hour)
	if first.Before(cutoff) && !times[7].Before(cutoff) {
		span := cutoff.Add(-time.Microsecond).Sub(first)
		for stage := 1; stage < len(times); stage++ {
			times[stage] = first.Add(span * time.Duration(stage) / 7)
		}
	}
	return times
}

func (r *runner) claims(ctx context.Context) error {
	var plans []claimPlan
	var events []claimEvent
	for i, p := range r.plans {
		for j, loss := range p.Losses {
			rng := entityRNG(r.cfg.Seed, i, 100+j)
			plan := claimPlan{Policy: i, Index: j, Loss: loss, Reject: rng.Intn(100) < 8, Amount: int64(300000 + rng.Intn(4700000)), Workshop: rng.Intn(len(r.refs.workshops))}
			if hasDeliberateClaims(i) {
				plan.Workshop = 0
			}
			n := len(plans)
			plans = append(plans, plan)
			reportAt := loss.AddDate(0, 0, rng.Intn(3))
			if i <= 1 && j == 2 {
				reportAt = loss // Küçük N'deki kasıtlı atölye patlaması anchor içinde kalır.
			}
			for stage, at := range claimTimes(r.cfg, reportAt, rng) {
				if !at.After(r.cfg.Anchor) {
					events = append(events, claimEvent{At: at, Plan: n, Stage: stage})
				}
			}
		}
	}
	sort.Slice(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if !a.At.Equal(b.At) {
			return a.At.Before(b.At)
		}
		if a.Plan != b.Plan {
			return a.Plan < b.Plan
		}
		return a.Stage < b.Stage
	})
	if err := r.progress(ctx, "CLAIMS", 0, len(events)); err != nil {
		return err
	}
	for i, event := range events {
		if err := r.claimEvent(ctx, &plans[event.Plan], event); err != nil {
			return fmt.Errorf("hasar poliçe=%d sıra=%d aşama=%d: %w", plans[event.Plan].Policy+1, plans[event.Plan].Index, event.Stage, err)
		}
		if (i+1)%1000 == 0 || i+1 == len(events) {
			if err := r.progress(ctx, "CLAIMS", i+1, len(events)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *runner) claimEvent(ctx context.Context, p *claimPlan, e claimEvent) error {
	if p.Hold {
		return nil
	}
	policy := r.plans[p.Policy]
	at := e.At.Add(12 * time.Hour)
	svc := claim.NewService(r.pool, func() time.Time { return at }, r.cfg.Fraud, r.cfg.Workflow)
	adj := r.refs.adjusters[p.Policy%len(r.refs.adjusters)]
	adjctx := r.eventContext(ctx, adj, auth.RoleClaimsAdjuster, e.At)
	change := func(to string) error {
		c, err := svc.ChangeStatus(adjctx, p.Claim.ID, claim.TransitionRequest{ToStatus: to, Note: "Kurgusal SIM dosya işlemi", ExpectedRowVersion: p.Claim.RowVersion})
		if err == nil {
			p.Claim = c
		}
		return err
	}
	switch e.Stage {
	case 0:
		code := pricingCoverage(policy.Product)
		var workshop *int64
		if policy.Product.IsVehicle() {
			workshop = &r.refs.workshops[p.Workshop]
		}
		c, err := svc.Report(adjctx, claim.ReportRequest{PolicyID: policy.ID, CoverageCode: code, LossDate: caldate.Format(p.Loss), Description: fmt.Sprintf("Kurgusal SIM hasar %06d/%d", p.Policy+1, p.Index), ClaimedAmountKurus: &p.Amount, WorkshopID: workshop})
		if err != nil {
			return err
		}
		p.Claim = c
		if c.Status == "FRAUD_REVIEW" {
			if !at.Before(r.cfg.openSince()) {
				p.Hold = true
			} else if entityRNG(r.cfg.Seed, p.Policy, 200+p.Index).Intn(100) < 80 {
				p.Reject = true
			}
		}
	case 1:
		if p.Reject {
			return change("REJECTED")
		}
		if p.Claim.Status == "REPORTED" {
			return change("TRIAGED")
		}
	case 2:
		if !p.Reject {
			return change("APPRAISAL")
		}
	case 3:
		if !p.Reject {
			uid := r.refs.appraisers[p.Policy%len(r.refs.appraisers)]
			_, err := svc.Appraise(r.eventContext(ctx, uid, auth.RoleAppraiser, e.At), p.Claim.ID, claim.AppraisalRequest{DamageAmountKurus: &p.Amount, Notes: "Kurgusal SIM ekspertizi", InspectedAt: at.Format(time.RFC3339)})
			return err
		}
	case 4:
		if !p.Reject {
			return change("ADJUSTMENT")
		}
	case 5:
		if !p.Reject {
			if _, err := svc.Approve(adjctx, p.Claim.ID); err != nil {
				return err
			}
			p.Claim.Status = "APPROVED"
			p.Claim.RowVersion++
		}
	case 6:
		if !p.Reject {
			if _, err := svc.Pay(r.eventContext(ctx, r.refs.account, auth.RoleAccounting, e.At), p.Claim.ID); err != nil {
				return err
			}
			p.Claim.Status = "PAID"
			p.Claim.RowVersion++
		}
	case 7:
		return change("CLOSED")
	}
	return nil
}

func pricingCoverage(product pricing.Product) string {
	for _, coverage := range pricing.Catalog[product].Coverages {
		if coverage.Required {
			return coverage.Code
		}
	}
	return ""
}
