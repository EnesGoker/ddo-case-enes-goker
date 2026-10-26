//go:build integration

package app_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/claim"
	"github.com/EnesGoker/guvenpolice/backend/internal/fraud"
	"github.com/EnesGoker/guvenpolice/backend/internal/platform/db"
	"github.com/EnesGoker/guvenpolice/backend/internal/seed"
	"github.com/EnesGoker/guvenpolice/backend/internal/workflow"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppraisalBusinessKeyRecovery(t *testing.T) {
	defer assertBillingInvariant(t)
	adjuster := tokenFor(t, "hasar@guvenpolice.test")
	expert := tokenFor(t, "eksper@guvenpolice.test")
	for _, recovery := range []string{"missing_response", "stale_response"} {
		t.Run(recovery, func(t *testing.T) {
			pol := claimPolicy(t)
			c := reportClaim(t, adjuster, pol, "KASKO_ANA", time.Now().Format("2006-01-02"))
			c = transitionClaim(t, adjuster, c, "TRIAGED")
			c = transitionClaim(t, adjuster, c, "APPRAISAL")
			path := claimPath(c) + "/appraisals"
			key := fmt.Sprintf("a81-appraisal-%v-%s", c["id"], recovery)
			inspected := time.Now().UTC().Truncate(time.Second)
			if recovery == "stale_response" {
				inspected = inspected.Add(123456789 * time.Nanosecond)
			}
			body := map[string]any{
				"damage_amount_kurus": 120000,
				"inspected_at":        inspected.Format(time.RFC3339Nano),
				"notes":               "Çökme sonrası aynı iş kaydı bulunmalı",
			}
			s, first, _ := postKey(t, expert, path, body, key)
			require.Equal(t, http.StatusCreated, s, first)
			before := financialCounts(t)
			ctx := context.Background()
			if recovery == "missing_response" {
				_, err := pool.Exec(ctx, `
					DELETE FROM idempotency_keys WHERE idem_key=$1`, key)
				require.NoError(t, err)
			} else {
				_, err := pool.Exec(ctx, `
					UPDATE idempotency_keys
					   SET status='IN_PROGRESS', response_status=NULL, response_body=NULL,
					       completed_at=NULL, created_at=now()-interval '1 hour'
					 WHERE idem_key=$1`, key)
				require.NoError(t, err)
			}
			s, replay, _ := postKey(t, expert, path, body, key)
			require.Equal(t, http.StatusCreated, s, replay)
			assertSameAppraisal(t, first, replay)
			var appraisals, audits int
			require.NoError(t, pool.QueryRow(ctx, `
				SELECT (SELECT count(*) FROM appraisals WHERE claim_id=$1),
				       (SELECT count(*) FROM audit_log
				         WHERE entity_type='claim' AND entity_id=$2 AND action='CLAIM_APPRAISED')`,
				num(c["id"]), fmt.Sprint(num(c["id"]))).Scan(&appraisals, &audits))
			assert.Equal(t, 1, appraisals)
			assert.Equal(t, 1, audits)
			assert.Equal(t, before, financialCounts(t), "kurtarma yeni iş veya yan etki üretmemeli")
		})
	}
}

func TestAppraisalDistinctKeysAndUnkeyedServicePreserveAppend(t *testing.T) {
	defer assertBillingInvariant(t)
	adjuster := tokenFor(t, "hasar@guvenpolice.test")
	expert := tokenFor(t, "eksper@guvenpolice.test")
	c := reportClaim(t, adjuster, claimPolicy(t), "KASKO_ANA", time.Now().Format("2006-01-02"))
	c = transitionClaim(t, adjuster, c, "TRIAGED")
	c = transitionClaim(t, adjuster, c, "APPRAISAL")
	path := claimPath(c) + "/appraisals"
	key := fmt.Sprintf("a81-append-%v", c["id"])
	inspected := time.Now().UTC().Truncate(time.Second)
	body := map[string]any{"damage_amount_kurus": 120000, "inspected_at": inspected.Format(time.RFC3339)}
	s, first, _ := postKey(t, expert, path, body, key)
	require.Equal(t, http.StatusCreated, s, first)
	body["damage_amount_kurus"] = 240000
	s, second, _ := postKey(t, expert, path, body, key+"-new")
	require.Equal(t, http.StatusCreated, s, second)
	assert.NotEqual(t, first["id"], second["id"])
	ctx := principalContext(t, "eksper@guvenpolice.test", auth.RoleAppraiser)
	amount := int64(360000)
	service := claim.NewService(pool, time.Now, fraud.DefaultConfig(), workflow.DefaultConfig())
	third, err := service.Appraise(ctx, num(c["id"]), claim.AppraisalRequest{
		DamageAmountKurus: &amount, InspectedAt: inspected.Format(time.RFC3339),
	})
	require.NoError(t, err)
	var unkeyed bool
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT request_key IS NULL FROM appraisals WHERE id=$1`, third.ID).Scan(&unkeyed))
	assert.True(t, unkeyed, "anahtarsız servis yolu NULL yazmalı")
	c = transitionClaim(t, adjuster, c, "ADJUSTMENT")
	_, err = pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE idem_key=$1`, key)
	require.NoError(t, err)
	body["damage_amount_kurus"] = 120000
	s, recovered, _ := postKey(t, expert, path, body, key)
	require.Equal(t, http.StatusCreated, s, recovered, "durum kontrolünden önce eski iş bulunmalı")
	assertSameAppraisal(t, first, recovered)
	s, preview := newClient(t).do(http.MethodPost, claimPath(c)+"/settlement/preview", adjuster, nil)
	require.Equal(t, http.StatusOK, s, preview)
	assert.Equal(t, amount, num(preview["gross_damage_kurus"]), "eski anahtar kurtarması en son ekspertizi değiştirmemeli")
	_, err = pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE idem_key=$1`, key)
	require.NoError(t, err)
	body["damage_amount_kurus"] = 999000
	s, mismatch, _ := postKey(t, expert, path, body, key)
	assert.Equal(t, http.StatusUnprocessableEntity, s, mismatch)
	assert.Equal(t, "IDEMPOTENCY_KEY_REUSED", mismatch["error"].(map[string]any)["code"])
}

func assertSameAppraisal(t *testing.T, first, replay map[string]any) {
	t.Helper()
	firstTime, err := time.Parse(time.RFC3339Nano, first["inspected_at"].(string))
	require.NoError(t, err)
	replayTime, err := time.Parse(time.RFC3339Nano, replay["inspected_at"].(string))
	require.NoError(t, err)
	assert.True(t, firstTime.Truncate(time.Microsecond).Equal(replayTime.Truncate(time.Microsecond)),
		"kurtarma aynı inceleme anını PostgreSQL hassasiyetinde korumalı")
	canonical := make(map[string]any, len(replay))
	for field, value := range replay {
		canonical[field] = value
	}
	canonical["inspected_at"] = first["inspected_at"]
	assert.Equal(t, first, canonical, "kurtarma aynı ekspertiz kaydını döndürmeli")
}

func TestAppraisalRequestKeyMigrationUpDownUp(t *testing.T) {
	ctx := context.Background()
	u, drop, err := createTempDB(ctx, os.Getenv("DATABASE_URL"), "gp_appraisal_key")
	require.NoError(t, err)
	defer drop()
	require.NoError(t, db.Migrate(ctx, u, migrationsDir))
	p, err := pgxpool.New(ctx, u)
	require.NoError(t, err)
	defer p.Close()
	require.NoError(t, seed.Demo(ctx, p, demoPassword))
	var claimID, actorID, legacyID int64
	require.NoError(t, p.QueryRow(ctx, `SELECT id FROM claims ORDER BY id LIMIT 1`).Scan(&claimID))
	require.NoError(t, p.QueryRow(ctx, `SELECT id FROM users WHERE role='APPRAISER' ORDER BY id LIMIT 1`).Scan(&actorID))
	require.NoError(t, p.QueryRow(ctx, `
		INSERT INTO appraisals (claim_id,appraiser_id,inspected_at,damage_amount_kurus)
		VALUES ($1,$2,now(),1000) RETURNING id`, claimID, actorID).Scan(&legacyID))
	sqlDB, err := sql.Open("pgx", u)
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, goose.DownToContext(ctx, sqlDB, migrationsDir, 12))
	require.NoError(t, goose.UpContext(ctx, sqlDB, migrationsDir))
	var preserved bool
	require.NoError(t, p.QueryRow(ctx, `
		SELECT request_key IS NULL AND damage_amount_kurus=1000 FROM appraisals WHERE id=$1`, legacyID).Scan(&preserved))
	assert.True(t, preserved, "mevcut ekspertiz değişmeden ve NULL anahtarla kalmalı")
	_, err = p.Exec(ctx, `
		INSERT INTO appraisals (claim_id,appraiser_id,inspected_at,damage_amount_kurus,request_key)
		VALUES ($1,$2,now(),2000,'migration-key')`, claimID, actorID)
	require.NoError(t, err)
	_, err = p.Exec(ctx, `
		INSERT INTO appraisals (claim_id,appraiser_id,inspected_at,damage_amount_kurus,request_key)
		VALUES ($1,$2,now(),3000,'migration-key')`, claimID, actorID)
	assert.True(t, db.Violates(err, "appraisals_request_key_uq"), "aynı aktör ve dolu anahtar benzersiz olmalı")
	_, err = p.Exec(ctx, `
		INSERT INTO appraisals (claim_id,appraiser_id,inspected_at,damage_amount_kurus)
		VALUES ($1,$2,now(),4000),($1,$2,now(),5000)`, claimID, actorID)
	assert.NoError(t, err, "NULL anahtarlar birden çok ekspertize izin vermeli")
}
