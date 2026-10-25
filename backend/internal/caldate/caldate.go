// Package caldate takvim günü (saat bilgisi olmayan tarih) yardımcılarıdır.
// Tarihler UTC gece yarısı time.Time olarak taşınır (pgx DATE ile aynı); "bugün" Türkiye saatine göredir.
package caldate

import (
	"fmt"
	"time"
)

const Layout = "2006-01-02"

// Türkiye 2016'dan beri sabit UTC+3 (yaz saati yok). Distroless imajda tzdata olmadığı için sabit bölge.
var turkey = time.FixedZone("TRT", 3*60*60)

// Parse "YYYY-MM-DD" biçimini katı olarak çözer (ör. 2026-02-29 reddedilir).
func Parse(s string) (time.Time, error) {
	t, err := time.Parse(Layout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("tarih YYYY-MM-DD biçiminde olmalı: %q", s)
	}
	return t, nil
}

// Today verilen anın Türkiye saatindeki takvim günüdür.
func Today(now time.Time) time.Time {
	y, m, d := now.In(turkey).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Of herhangi bir time.Time'ın takvim gününü (UTC) döner.
func Of(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// AgeOn doğum tarihinden on gününe kadar tamamlanmış yaş.
// 29 Şubat doğumlular artık olmayan yıllarda yaşını 1 Mart'ta tamamlar (varsayım).
func AgeOn(birth, on time.Time) int {
	age := on.Year() - birth.Year()
	if on.Month() < birth.Month() || (on.Month() == birth.Month() && on.Day() < birth.Day()) {
		age--
	}
	return age
}

// AddYear bir yıl sonrasını döner. 29 Şubat + 1 yıl = 1 Mart (Go normalizasyonu; varsayım).
func AddYear(t time.Time) time.Time {
	return t.AddDate(1, 0, 0)
}

func Format(t time.Time) string { return t.Format(Layout) }

// AddMonths başlangıç gününü hedef ayın son gününe kırpar; ara aylar sonucu etkilemez.
func AddMonths(t time.Time, months int) time.Time {
	first := time.Date(t.Year(), t.Month()+time.Month(months), 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(t.Day(), last), 0, 0, 0, 0, time.UTC)
}
