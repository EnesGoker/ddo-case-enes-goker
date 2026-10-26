//go:build integration

package app_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/payment"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/httpx"
	"github.com/EnesGoker/guvenpolice/backend/internal/policy"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionScopeUsesExistingConnectionAtPoolCapacity(t *testing.T) {
	defer assertBillingInvariant(t)
	for _, kind := range []string{"endorsement", "cancel", "vehicle_preview", "cancel_preview", "customer_pay", "customer_billing"} {
		t.Run(kind, func(t *testing.T) {
			pol := billingPolicy(t, 0, 1, 300000)
			email, role := "acente@guvenpolice.test", auth.RoleAgent
			if kind == "customer_pay" || kind == "customer_billing" {
				email, role = "musteri@guvenpolice.test", auth.RoleCustomer
			}
			actor := principalContext(t, email, role)
			cfg, err := pgxpool.ParseConfig(pool.Config().ConnString())
			require.NoError(t, err)
			cfg.MaxConns = 4
			cfg.MinConns = 0
			var entered atomic.Int32
			release := make(chan struct{})
			// İlk dört çağrı bağlantıyı almadan gövdeye geçmez: hepsi transaction için
			// bir bağlantı tuttuğunda havuzda ek yetki sorgusuna boş yer kalmaz.
			cfg.BeforeAcquire = func(ctx context.Context, _ *pgx.Conn) bool {
				n := entered.Add(1)
				if n == 4 {
					close(release)
				}
				if n <= 4 {
					select {
					case <-release:
					case <-ctx.Done():
						return false
					}
				}
				return true
			}
			small, err := pgxpool.NewWithConfig(context.Background(), cfg)
			require.NoError(t, err)
			defer small.Close()
			ctx, cancel := context.WithTimeout(actor, 5*time.Second)
			defer cancel()
			calls := make(chan error, 4)
			for range 4 {
				go func() {
					changes := policy.NewChangeService(small, time.Now, 1000)
					vehicle := policy.VehicleChangeRequest{EffectiveDate: day(dateOf(t, pol["start_date"]), 30), VehicleGroup: "D"}
					base := 1
					vehicle.BaseVersionNo = &base
					cancellation := policy.CancelRequest{EffectiveDate: day(dateOf(t, pol["start_date"]), 30), Reason: "CUSTOMER_REQUEST", BaseVersionNo: &base}
					var err error
					switch kind {
					case "endorsement":
						_, err = changes.VehicleChange(ctx, num(pol["id"]), vehicle)
					case "cancel":
						_, err = changes.Cancel(ctx, num(pol["id"]), cancellation)
					case "vehicle_preview":
						_, err = changes.PreviewVehicleChange(ctx, num(pol["id"]), vehicle)
					case "cancel_preview":
						_, err = changes.PreviewCancel(ctx, num(pol["id"]), cancellation)
					case "customer_pay":
						amount := int64(1)
						_, err = payment.NewService(small, time.Now, 1000).Pay(ctx, num(pol["id"]), payment.PayRequest{AmountKurus: &amount, Method: "CARD"})
					case "customer_billing":
						_, err = payment.NewService(small, time.Now, 1000).Billing(ctx, num(pol["id"]))
					}
					calls <- err
				}()
			}
			successes := 0
			for range 4 {
				err := <-calls
				if err == nil {
					successes++
					continue
				}
				var business *httpx.AppError
				if assert.True(t, errors.As(err, &business), "havuz doluyken teknik zaman aşımı olmamalı: %v", err) {
					switch kind {
					case "endorsement":
						assert.Equal(t, "STALE_VERSION", business.Code)
					case "cancel":
						assert.Equal(t, "POLICY_CANCELLED", business.Code)
					default:
						t.Errorf("okuma/ödeme hatası: %v", err)
					}
				}
			}
			expected := 4
			if kind == "endorsement" || kind == "cancel" {
				expected = 1
			}
			assert.Equal(t, expected, successes)
			assert.NoError(t, ctx.Err(), "ek bağlantı beklemeden tamamlanmalı")
		})
	}
}
