package simulation

import (
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/proration"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestPolicyPlanRepeatabilityAndRanges(t *testing.T) {
	c := DefaultConfig()
	counts := map[string]int{}
	for i := range 1000 {
		p := planPolicy(c, i)
		assert.Equal(t, p, planPolicy(c, i))
		assert.False(t, p.Start.Before(c.Anchor.AddDate(-5, 0, 0)))
		assert.False(t, p.Start.After(c.Anchor.AddDate(0, 0, -15)))
		assert.Equal(t, caldate.AddYear(p.Start), p.End)
		if p.Product.IsVehicle() {
			_, ok := policy.NormalizePlate(p.Plate)
			assert.True(t, ok)
		}
		if p.CancelAt != nil {
			assert.Empty(t, p.Losses)
		}
		counts[string(p.Product)]++
	}
	assert.Equal(t, map[string]int{"KASKO": 500, "TRAFIK": 350, "KONUT": 150}, counts)
}

func TestDueBalanceNeverPrepaysFuture(t *testing.T) {
	at := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	v := payment.BillingView{Installments: []payment.InstallmentView{
		{DueDate: "2026-10-25", AmountKurus: 1000, PaidKurus: 300, Status: "PARTIAL"},
		{DueDate: "2026-10-26", AmountKurus: 2000, Status: "PENDING"},
		{DueDate: "2026-10-24", AmountKurus: 500, Status: "CANCELLED"},
	}, Interest: []payment.InterestView{{AmountKurus: 100, PaidKurus: 25}}}
	assert.Equal(t, int64(775), dueBalance(v, at))
}

func TestLargePlanAnnualIncidence(t *testing.T) {
	cfg := DefaultConfig()
	type exposure struct {
		days   int64
		claims int64
	}
	totals := map[string]exposure{}
	for i := range 100000 {
		p := planPolicy(cfg, i)
		end := minDate(p.End, cfg.Anchor)
		if p.CancelAt != nil {
			end = minDate(end, *p.CancelAt)
		}
		value := totals[string(p.Product)]
		value.days += proration.DaysBetween(p.Start, end)
		value.claims += int64(len(p.Losses))
		totals[string(p.Product)] = value
		for _, loss := range p.Losses {
			if loss.Before(p.Start) || !loss.Before(p.End) || loss.After(cfg.Anchor) {
				t.Fatalf("dönem dışı hasar: %+v %s", p, loss)
			}
		}
	}
	for product, expected := range map[string]int64{"KASKO": 1200, "TRAFIK": 600, "KONUT": 400} {
		value := totals[product]
		bp := value.claims * 365 * 10000 / value.days
		t.Logf("%s: claims=%d exposure_days=%d annual_incidence_bp=%d", product, value.claims, value.days, bp)
		assert.InDelta(t, expected, bp, 150, product)
	}
}
