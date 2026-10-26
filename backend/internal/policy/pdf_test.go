package policy

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/EnesGoker/guvenpolice/backend/internal/auth"
	"github.com/EnesGoker/guvenpolice/backend/internal/policypdf"
	"github.com/EnesGoker/guvenpolice/backend/internal/pricing"
)

func TestPDFDocumentPrivateFieldsFollowBillingRoles(t *testing.T) {
	plan := &policypdf.Plan{Total: 123456}
	for _, role := range []auth.Role{auth.RoleCustomer, auth.RoleAgent, auth.RoleUnderwriter, auth.RoleAccounting} {
		d := pdfDocument(Detail{}, VersionView{}, role, "*******1234", plan, time.Time{})
		require.Equal(t, "*******1234", d.MaskedID, role)
		require.Same(t, plan, d.Billing, role)
	}
	for _, role := range []auth.Role{auth.RoleClaimsAdjuster, auth.RoleAppraiser, ""} {
		d := pdfDocument(Detail{}, VersionView{}, role, "*******1234", plan, time.Time{})
		require.Empty(t, d.MaskedID, role)
		require.Nil(t, d.Billing, role)
	}
}

func TestPDFPremiumLinesUseSavedEndorsementOrExplicitAbsence(t *testing.T) {
	id := int64(2)
	want := []pricing.Line{{Label: "Yaş katsayısı", MultiplierBP: 11000, AmountKurus: 1234}}
	raw, err := json.Marshal(CalcDetail{NewPremiumLines: want})
	require.NoError(t, err)
	d := Detail{Endorsements: []EndorsementView{{ToVersionID: &id, CalcDetail: raw}}}
	lines, err := pdfPremiumLines(context.Background(), nil, d, VersionView{ID: id, VersionNo: 2})
	require.NoError(t, err)
	require.Equal(t, want, lines)
	lines, err = pdfPremiumLines(context.Background(), nil, Detail{}, VersionView{VersionNo: 1})
	require.NoError(t, err)
	require.Empty(t, lines, "quote_id=NULL için tarife çağrısı veya tarihsel yeniden hesap yok")
	d.Endorsements[0].CalcDetail = json.RawMessage(`{`)
	_, err = pdfPremiumLines(context.Background(), nil, d, VersionView{ID: id, VersionNo: 2})
	require.Error(t, err, "bozuk kaydı sessizce özgün döküm gibi sunma")
}
