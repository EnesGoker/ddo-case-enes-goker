package seed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
	"github.com/EnesGoker/guvenpolice/backend/internal/claim"
	"github.com/EnesGoker/guvenpolice/backend/internal/fraud"
	"github.com/EnesGoker/guvenpolice/backend/internal/idempotency"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/clock"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	Now                  func() time.Time
	Fraud                fraud.Config
	Workflow             workflow.Config
	LateInterestDailyPPM int64
}

func (o Options) defaults() Options {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Fraud == (fraud.Config{}) {
		o.Fraud = fraud.DefaultConfig()
	}
	if o.Workflow == (workflow.Config{}) {
		o.Workflow = workflow.DefaultConfig()
	}
	if o.LateInterestDailyPPM == 0 {
		o.LateInterestDailyPPM = 1000
	}
	return o
}

// fraudScenario var olan hizmet yollarını kullanır. Her adımın sabit iş anahtarı,
// kısmen tamamlanan ilk seed'in yeniden çalıştırılmasını da güvenli kılar.
func fraudScenario(ctx context.Context, pool *pgxpool.Pool, options Options) error {
	options = options.defaults()
	if err := options.Fraud.Validate(); err != nil {
		return err
	}
	if err := options.Workflow.Validate(); err != nil {
		return err
	}
	today := caldate.Today(options.Now())
	var customerID, agentID, agentUser, accountUser, adjusterUser int64
	if err := pool.QueryRow(ctx, `
  SELECT c.id,a.id,a.user_id,
         (SELECT id FROM users WHERE email='muhasebe@guvenpolice.test'),
         (SELECT id FROM users WHERE email='hasar@guvenpolice.test')
    FROM customers c JOIN agents a ON a.id=c.agent_id WHERE c.national_id='10000000001'`).Scan(&customerID, &agentID, &agentUser, &accountUser, &adjusterUser); err != nil {
		return err
	}
	ensure := func(no string, days int) (policy.Issued, error) {
		var out policy.Issued
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			var start time.Time
			err := tx.QueryRow(ctx, `
    SELECT p.id,p.start_date,v.annual_premium_kurus FROM policies p
     JOIN policy_versions v ON v.policy_id=p.id AND v.version_no=1 WHERE p.policy_no=$1`, no).Scan(&out.ID, &start, &out.AnnualPremium)
			if err == nil {
				out.StartDate = caldate.Format(start)
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			out, err = issuePriced(ctx, tx, customerID, agentID, agentUser, no, "34FRD123", today.AddDate(0, 0, -days), 1)
			return err
		})
		return out, err
	}
	pay := func(p policy.Issued, key string) error {
		at, err := caldate.Parse(p.StartDate)
		if err != nil {
			return err
		}
		accountCtx := auth.WithPrincipal(clock.WithTime(ctx, at), auth.Principal{UserID: accountUser, Role: auth.RoleAccounting})
		_, err = payment.NewService(pool, options.Now, options.LateInterestDailyPPM).Pay(idempotency.WithKey(accountCtx, key), p.ID, payment.PayRequest{AmountKurus: &p.AnnualPremium, Method: "TRANSFER", PaymentDate: p.StartDate})
		return err
	}
	adjctx := auth.WithPrincipal(ctx, auth.Principal{UserID: adjusterUser, Role: auth.RoleClaimsAdjuster})
	claims := claim.NewService(pool, options.Now, options.Fraud, options.Workflow)
	report := func(p policy.Issued, loss time.Time, key string) (claim.Claim, error) {
		amount := int64(100000)
		return claims.Report(idempotency.WithKey(adjctx, key), claim.ReportRequest{PolicyID: p.ID, CoverageCode: "KASKO_ANA", LossDate: caldate.Format(loss), Description: "Kurgusal hasar demo senaryosu", ClaimedAmountKurus: &amount})
	}
	first, err := ensure("GP-DEMO-FP-1", 300)
	if err != nil {
		return err
	}
	if err := pay(first, "seed-a6-fp1-payment"); err != nil {
		return err
	}
	start, err := caldate.Parse(first.StartDate)
	if err != nil {
		return err
	}
	if _, err := report(first, start.AddDate(0, 0, 50), "seed-a6-fp1-claim-1"); err != nil {
		return err
	}
	late, err := report(first, start.AddDate(0, 0, 260), "seed-a6-fp1-claim-2")
	if err != nil {
		return err
	}
	past := start.AddDate(0, 0, 296)
	pastClaims := claim.NewService(pool, func() time.Time { return past }, options.Fraud, options.Workflow)
	for _, to := range []string{"TRIAGED", "APPRAISAL"} {
		if (to == "TRIAGED" && late.Status != "REPORTED") || (to == "APPRAISAL" && late.Status != "TRIAGED") {
			continue
		}
		late, err = pastClaims.ChangeStatus(clock.WithTime(adjctx, past), late.ID, claim.TransitionRequest{ToStatus: to, Note: "Kurgusal geçmiş SLA örneği", ExpectedRowVersion: late.RowVersion})
		if err != nil {
			return err
		}
	}
	agentCtx := auth.WithPrincipal(ctx, auth.Principal{UserID: agentUser, Role: auth.RoleAgent})
	_, err = policy.NewChangeService(pool, options.Now, options.LateInterestDailyPPM).Cancel(idempotency.WithKey(clock.WithTime(agentCtx, options.Now()), "seed-a6-fp1-cancel"), first.ID, policy.CancelRequest{EffectiveDate: caldate.Format(start.AddDate(0, 0, 294)), Reason: "CUSTOMER_REQUEST", Note: "Kurgusal yeni poliçe senaryosu"})
	if err != nil {
		return err
	}
	second, err := ensure("GP-DEMO-FP-2", 5)
	if err != nil {
		return err
	}
	if err := pay(second, "seed-a6-fp2-payment"); err != nil {
		return err
	}
	secondStart, err := caldate.Parse(second.StartDate)
	if err != nil {
		return err
	}
	if _, err := report(second, secondStart.AddDate(0, 0, 3), "seed-a6-fp2-claim-1"); err != nil {
		return fmt.Errorf("FP-2 hasarı: %w", err)
	}
	return nil
}
