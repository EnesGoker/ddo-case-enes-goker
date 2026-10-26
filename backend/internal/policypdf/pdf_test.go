package policypdf

import (
	"math"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFormatTLExactIntegerBounds(t *testing.T) {
	for _, tc := range []struct {
		in   int64
		want string
	}{
		{0, "0,00 TL"}, {1, "0,01 TL"}, {99, "0,99 TL"}, {100, "1,00 TL"},
		{123456, "1.234,56 TL"}, {-123456, "−1.234,56 TL"},
		{math.MaxInt64, "92.233.720.368.547.758,07 TL"},
		{math.MinInt64, "−92.233.720.368.547.758,08 TL"},
	} {
		require.Equal(t, tc.want, FormatTL(tc.in))
	}
}

func TestRenderUnicodeAndPagination(t *testing.T) {
	d := Document{PolicyNo: "GP-TEST-1", Product: "KONUT", Customer: "Çağrı Şen İğdır Öztürk Ünal",
		Start: "2026-01-01", End: "2027-01-01", VersionFrom: "2026-01-01", VersionNo: 1,
		Annual: 123456, Net: 123456, GeneratedAt: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)}
	for i := 0; i < 90; i++ {
		d.Coverages = append(d.Coverages, Coverage{Code: "CAM", Name: strings.Repeat("Türkçe açıklama çğıöşü ÇĞİÖŞÜ ", 5), Limit: 500000})
	}
	raw, err := Render(d)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(raw), "%PDF-"))
	require.Contains(t, string(raw), "/FontFile2")
	require.Contains(t, string(raw), "/ToUnicode")
	pages := regexp.MustCompile(`/Type\s*/Page\b`).FindAll(raw, -1)
	require.Greater(t, len(pages), 2, "uzun içerik kırpılmak yerine yeni sayfaya taşınır")
	require.Contains(t, FontLicense, "SIL OPEN FONT LICENSE Version 1.1")
}
