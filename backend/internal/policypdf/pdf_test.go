package policypdf

import (
	"bytes"
	"compress/zlib"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
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
		{123456, "1.234,56 TL"}, {-1, "-0,01 TL"}, {-99, "-0,99 TL"}, {-100, "-1,00 TL"},
		{-123456, "-1.234,56 TL"},
		{math.MaxInt64, "92.233.720.368.547.758,07 TL"},
		{math.MinInt64 + 1, "-92.233.720.368.547.758,07 TL"},
		{math.MinInt64, "-92.233.720.368.547.758,08 TL"},
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

// POLICY_PDF_SAMPLE=/tmp/ornek.pdf writes this document for a visual check of the rendered glyphs.
func TestRenderTurkishGlyphsAndNegativeAmount(t *testing.T) {
	d := Document{PolicyNo: "GP-TEST-2", Product: "KONUT", Customer: "ÇĞİÖŞÜ çğıöşü",
		Start: "2026-01-01", End: "2027-01-01", VersionFrom: "2026-01-01", VersionNo: 2,
		Annual: 123456, Initial: 246912, Delta: -123456, Written: 123456, Net: -123456,
		GeneratedAt: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)}
	raw, err := Render(d)
	require.NoError(t, err)
	if p := os.Getenv("POLICY_PDF_SAMPLE"); p != "" {
		require.NoError(t, os.WriteFile(p, raw, 0o644))
	}
	require.True(t, strings.HasPrefix(string(raw), "%PDF-"))
	require.NotContains(t, string(raw), "TCKN")

	// Every component of every composite glyph in the embedded subset must carry an outline.
	m := regexp.MustCompile(`/Length (\d+)\n/Filter /FlateDecode\n/Length1 \d+\n>>\nstream\n`).FindSubmatchIndex(raw)
	require.NotNil(t, m, "gömülü TrueType alt kümesi")
	n, err := strconv.Atoi(string(raw[m[2]:m[3]]))
	require.NoError(t, err)
	zr, err := zlib.NewReader(bytes.NewReader(raw[m[1] : m[1]+n]))
	require.NoError(t, err)
	subset, err := io.ReadAll(zr)
	require.NoError(t, err)
	glyphs, err := glyphData(subset)
	require.NoError(t, err)
	composites := 0
	for id, g := range glyphs {
		comps, _, err := parseComposite(g)
		require.NoError(t, err)
		if comps != nil {
			composites++
		}
		for _, c := range comps {
			require.NotEmpty(t, glyphs[c.glyph], "glif %d bileşeni %d alt kümede yok", id, c.glyph)
		}
	}
	require.Positive(t, composites, "Ö, Ğ, İ, Ü bileşik gliflerdir")
}

func TestFlattenCompositesRemovesNestedReferences(t *testing.T) {
	before, err := glyphData(font)
	require.NoError(t, err)
	flat, err := pdfFont()
	require.NoError(t, err)
	after, err := glyphData(flat)
	require.NoError(t, err)
	require.Len(t, after, len(before))
	nested := func(glyphs [][]byte) int {
		count := 0
		for _, g := range glyphs {
			comps, _, err := parseComposite(g)
			require.NoError(t, err)
			for _, c := range comps {
				sub, _, err := parseComposite(glyphs[c.glyph])
				require.NoError(t, err)
				if sub != nil {
					count++
				}
			}
		}
		return count
	}
	require.Positive(t, nested(before), "Noto Ö/Ğ/İ/Ü işaretlerini iç içe bileşenle kurar")
	require.Zero(t, nested(after))
	for id := range before {
		if c, _, _ := parseComposite(before[id]); c == nil {
			require.Equal(t, before[id], after[id], "basit glif %d değişmez", id)
		}
	}
}
