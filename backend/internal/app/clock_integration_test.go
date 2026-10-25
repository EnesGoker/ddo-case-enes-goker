//go:build integration

package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPICreatedTimestampsUseRealNow(t *testing.T) {
	// HTTP yolunda bağlam saati ayarlanmaz. Başlangıcı gelecekteki poliçe de şimdi oluşturulur.
	before := time.Now()
	pol := issuePolicy(t, tokenFor(t, "acente@guvenpolice.test"))
	after := time.Now()
	var policyAt, versionAt, planAt, journalAt time.Time
	require.NoError(t, pool.QueryRow(context.Background(), `
      SELECT p.created_at,v.recorded_at,pl.created_at,e.created_at
        FROM policies p JOIN policy_versions v ON v.policy_id=p.id AND v.version_no=1
        JOIN installment_plans pl ON pl.policy_id=p.id
        JOIN journal_entries e ON e.ref_type='POLICY_ISSUE' AND e.ref_id=p.id
       WHERE p.id=$1`, num(pol["id"])).Scan(&policyAt, &versionAt, &planAt, &journalAt))
	for _, at := range []time.Time{policyAt, versionAt, planAt, journalAt} {
		assert.False(t, at.Before(before), "%s < %s", at, before)
		assert.False(t, at.After(after), "%s > %s", at, after)
	}
}
