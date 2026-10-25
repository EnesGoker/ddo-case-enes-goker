//go:build integration

package app_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/caldate"
)

func TestAdjustmentRequiresAppraisalOnEveryEntry(t *testing.T) {
	adjuster := tokenFor(t, "hasar@guvenpolice.test")
	appraiser := tokenFor(t, "eksper@guvenpolice.test")
	for _, from := range []string{"APPRAISAL", "FRAUD_REVIEW"} {
		for _, hasAppraisal := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/appraisal=%t", from, hasAppraisal), func(t *testing.T) {
				c := reportClaim(t, adjuster, claimPolicy(t), "KASKO_ANA", caldate.Format(caldate.Today(time.Now())))
				c = transitionClaim(t, adjuster, c, "TRIAGED")
				c = transitionClaim(t, adjuster, c, "APPRAISAL")
				if hasAppraisal {
					s, out := newClient(t).do(http.MethodPost, claimPath(c)+"/appraisals", appraiser, map[string]any{
						"damage_amount_kurus": 100000, "inspected_at": time.Now().UTC().Format(time.RFC3339),
					})
					require.Equal(t, http.StatusCreated, s, out)
				}
				if from == "FRAUD_REVIEW" {
					c = transitionClaim(t, adjuster, c, "FRAUD_REVIEW")
				}
				beforeStatus, before := get(t, adjuster, claimPath(c))
				require.Equal(t, http.StatusOK, beforeStatus, before)
				var auditBefore int
				require.NoError(t, pool.QueryRow(context.Background(), `
                    SELECT count(*) FROM audit_log WHERE entity_type='claim' AND entity_id=$1`, fmt.Sprint(num(c["id"]))).Scan(&auditBefore))
				s, out := newClient(t).do(http.MethodPost, claimPath(c)+"/transition", adjuster, map[string]any{
					"to_status": "ADJUSTMENT", "expected_row_version": c["row_version"],
				})
				if hasAppraisal {
					require.Equal(t, http.StatusOK, s, out)
					assert.Equal(t, "ADJUSTMENT", out["status"])
					return
				}
				require.Equal(t, http.StatusConflict, s, out)
				assert.Equal(t, "APPRAISAL_REQUIRED", errCode(out))
				afterStatus, after := get(t, adjuster, claimPath(c))
				require.Equal(t, http.StatusOK, afterStatus, after)
				assert.Equal(t, before["status"], after["status"])
				assert.Equal(t, before["row_version"], after["row_version"])
				assert.Equal(t, before["history"], after["history"])
				var auditAfter, adjustmentTasks int
				require.NoError(t, pool.QueryRow(context.Background(), `
                    SELECT count(*) FROM audit_log WHERE entity_type='claim' AND entity_id=$1`, fmt.Sprint(num(c["id"]))).Scan(&auditAfter))
				assert.Equal(t, auditBefore, auditAfter)
				require.NoError(t, pool.QueryRow(context.Background(), `
                    SELECT count(*) FROM claim_tasks WHERE claim_id=$1 AND task_type='ADJUSTMENT'`, num(c["id"])).Scan(&adjustmentTasks))
				assert.Zero(t, adjustmentTasks)
			})
		}
	}
}
