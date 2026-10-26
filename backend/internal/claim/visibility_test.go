package claim

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExternalHistoryHidesInternalReviewAndKeepsRejection(t *testing.T) {
	actor := int64(7)
	from, internal, rejection := "FRAUD_REVIEW", "fraud skoru ve iç not", "Teminat kapsamı dışında"
	d := Detail{Claim: Claim{Status: from}, History: []History{
		{ToStatus: "TRIAGED", Note: &internal},
		{ToStatus: from, ActorUserID: &actor, Note: &internal},
		{FromStatus: &from, ToStatus: "APPRAISAL", ActorUserID: &actor, Note: &internal},
		{FromStatus: &from, ToStatus: "REJECTED", ActorUserID: &actor, Note: &rejection},
	}}
	for _, role := range []auth.Role{auth.RoleCustomer, auth.RoleAgent} {
		out := visibleDetail(d, role)
		assert.Equal(t, "UNDER_REVIEW", out.Status)
		raw, err := json.Marshal(out)
		require.NoError(t, err)
		assert.NotContains(t, strings.ToLower(string(raw)), "fraud")
		assert.Equal(t, &rejection, out.History[3].Note)
		for _, h := range out.History[:3] {
			assert.Nil(t, h.Note)
		}
	}
	assert.Equal(t, d, visibleDetail(d, auth.RoleClaimsAdjuster))
	assert.Equal(t, &internal, d.History[0].Note, "projeksiyon iç yanıtı değiştirmez")
}
