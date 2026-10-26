// Package policypdf renders an already authorized policy snapshot without DB or HTTP access.
package policypdf

import (
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/signintech/gopdf"
)

//go:embed fonts/NotoSans-Regular.ttf
var font []byte

// FontLicense retains the font's redistribution notice in the binary as well as the source tree.
//
//go:embed fonts/OFL.txt
var FontLicense string

type Coverage struct {
	Code, Name, DeductibleType string
	Limit, Deductible, Premium int64
	EventLimit                 *int64
}

// Document holds only what the policy detail endpoint returns: no installments,
// premium factor breakdown or national ID.
type Document struct {
	PolicyNo, Product, Customer, Status          string
	Start, End, CancelDate, CancelReason         string
	VersionFrom, VersionTo, Plate                string
	VersionNo, TariffNo                          int
	Annual, Initial, Delta, Written, Refund, Net int64
	AnnualLimit                                  *int64
	Coverages                                    []Coverage
	GeneratedAt                                  time.Time
}

// FormatTL uses integer arithmetic, including for math.MinInt64.
// The sign is ASCII "-": U+2212 is not drawn by the embedded font.
func FormatTL(kurus int64) string {
	sign := ""
	n := uint64(kurus)
	if kurus < 0 {
		sign = "-"
		n = uint64(-(kurus + 1)) + 1
	}
	s := strconv.FormatUint(n/100, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "." + s[i:]
	}
	return fmt.Sprintf("%s%s,%02d TL", sign, s, n%100)
}

func basisPoints(bp int64) string {
	return fmt.Sprintf("%d,%02d%%", bp/100, bp%100)
}

// Render creates one document per call; mutable GoPDF state is never shared between requests.
func Render(d Document) ([]byte, error) {
	fontData, err := pdfFont()
	if err != nil {
		return nil, err
	}
	p := &gopdf.GoPdf{}
	p.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := p.AddTTFFontByReader("NotoSans", bytes.NewReader(fontData)); err != nil {
		return nil, err
	}
	p.SetInfo(gopdf.PdfInfo{Title: "Poliçe özeti " + d.PolicyNo, Author: "GüvenPoliçe", Creator: "GüvenPoliçe", Subject: FontLicense, CreationDate: d.GeneratedAt})
	w := writer{pdf: p, policy: d.PolicyNo}
	if err := w.page(); err != nil {
		return nil, err
	}
	write := func(s string) { w.text(s, 10) }
	w.text("Poliçe özeti", 22)
	write("Kurgusal demo belgesi - son kayıtlı sürüm; resmi sigorta sözleşmesi değildir.")
	write("Üretim zamanı: " + d.GeneratedAt.UTC().Format("2006-01-02 15:04:05 UTC"))
	w.section("Sigortalı ve poliçe")
	write("Sigortalı: " + d.Customer)
	write("Poliçe: " + d.PolicyNo + " | Ürün: " + d.Product + " | Kayıt durumu: " + d.Status)
	write("Poliçe dönemi: " + d.Start + " / " + d.End + " (bitiş hariç)")
	if d.CancelDate != "" {
		write("İptal etki tarihi: " + d.CancelDate + " | Gerekçe: " + d.CancelReason)
	}
	end := d.VersionTo
	if end == "" {
		end = d.End
	}
	write(fmt.Sprintf("Son kayıtlı sürüm: %d | Tarife sürümü: %d | Sürüm aralığı: %s / %s", d.VersionNo, d.TariffNo, d.VersionFrom, end))
	if d.Plate != "" {
		write("Plaka: " + d.Plate)
	}
	w.section("Prim özeti - yazılan prim")
	write("Sürüm yıllık primi: " + FormatTL(d.Annual))
	write("İlk yazılan prim: " + FormatTL(d.Initial) + " | Zeyil farkları: " + FormatTL(d.Delta))
	write("Yazılan prim: " + FormatTL(d.Written) + " | İptal iadesi: " + FormatTL(d.Refund))
	w.text("Yazılan net prim: "+FormatTL(d.Net), 14)
	w.section("Teminat, limit ve muafiyet")
	if d.AnnualLimit != nil {
		write("Yıllık toplam limit: " + FormatTL(*d.AnnualLimit))
	}
	for _, c := range d.Coverages {
		event := "Yok"
		if c.EventLimit != nil {
			event = FormatTL(*c.EventLimit)
		}
		deductible := "Yok"
		switch c.DeductibleType {
		case "FIXED":
			deductible = FormatTL(c.Deductible)
		case "PERCENT_BP":
			deductible = basisPoints(c.Deductible)
		}
		write(c.Name + " (" + c.Code + ")")
		write("Dönem limiti: " + FormatTL(c.Limit) + " | Olay başı: " + event)
		write("Muafiyet: " + deductible + " | Teminat yıllık primi: " + FormatTL(c.Premium))
	}
	if w.err != nil {
		return nil, w.err
	}
	return p.GetBytesPdfReturnErr()
}

type writer struct {
	pdf    *gopdf.GoPdf
	policy string
	y      float64
	pages  int
	err    error
}

func (w *writer) page() error {
	w.pdf.AddPage()
	w.pages++
	w.pdf.SetTextColor(16, 36, 58)
	if err := w.pdf.SetFont("NotoSans", "", 11); err != nil {
		return err
	}
	w.pdf.SetXY(40, 22)
	if err := w.pdf.Cell(&gopdf.Rect{W: 515, H: 18}, "GüvenPoliçe | "+w.policy); err != nil {
		return err
	}
	w.pdf.SetStrokeColor(13, 148, 136)
	w.pdf.SetLineWidth(1)
	w.pdf.Line(40, 47, 555, 47)
	w.pdf.SetXY(40, 805)
	if err := w.pdf.SetFont("NotoSans", "", 8); err != nil {
		return err
	}
	if err := w.pdf.Cell(&gopdf.Rect{W: 515, H: 14}, fmt.Sprintf("Kurgusal demo verisi | Son kayıtlı sürüm | Sayfa %d", w.pages)); err != nil {
		return err
	}
	w.y = 61
	return nil
}

func (w *writer) section(s string) {
	if w.err != nil {
		return
	}
	// Keep a heading together with at least the first body line.
	if w.y+12+19+16 > 782 {
		w.err = w.page()
	}
	w.y += 12
	w.text(s, 13)
}

func (w *writer) text(s string, size float64) {
	if w.err != nil || s == "" {
		return
	}
	w.err = w.pdf.SetFont("NotoSans", "", size)
	if w.err != nil {
		return
	}
	lines, err := w.pdf.SplitTextWithWordWrap(strings.ReplaceAll(s, "\t", " "), 515)
	if err != nil {
		w.err = err
		return
	}
	for _, line := range lines {
		if w.y+size+6 > 782 {
			if w.err = w.page(); w.err != nil {
				return
			}
			if w.err = w.pdf.SetFont("NotoSans", "", size); w.err != nil {
				return
			}
		}
		w.pdf.SetXY(40, w.y)
		if w.err = w.pdf.Cell(&gopdf.Rect{W: 515, H: size + 6}, line); w.err != nil {
			return
		}
		w.y += size + 6
	}
}
