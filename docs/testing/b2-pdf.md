# B2 — Poliçe PDF kabul kaydı

**Durum: TAMAMLANDI.** 26 Ekim 2026, dal `bonus/police-pdf` (taban `ec70df6`). Uç `GET /api/v1/policies/{id}/pdf`; karar [ADR-28](../../DECISIONS.md#adr-28--poliçe-pdf-özeti-b2).

## Görsel kontrolde bulunan hatalar

| Hata | Kök neden | Çözüm | Test |
|---|---|---|---|
| Negatif tutar pozitif görünüyordu | U+2212 Noto'nun cmap tablosunda yok | `FormatTL` ASCII "-" kullanır | `TestFormatTLExactIntegerBounds` (-1, -100, MinInt64+1, MinInt64) |
| Ö, Ü, Ğ, İ işaretleri kayboluyordu; metin çıkarımı doğruydu | GoPDF v0.38.3 alt kümeye bileşenlerin yalnız ilk seviyesini alır; Noto bu harfleri iç içe kurar (Ö → O + glif 2660 → glif 106) | Gömülü font bellekte bir kez düzleştirilir; font dosyası değişmez | `TestRenderTurkishGlyphsAndNegativeAmount` alt kümede eksik bileşen arar; düzeltme geri alınınca "glif 2660 bileşeni 106 alt kümede yok" ile FAIL |

Görsel kontrol: ilk sayfa `qlmanage -t -s 1600` ile PNG'ye çevrildi ve açılıp incelendi; PNG'ler repoya alınmadı.
- Örnek belge: `POLICY_PDF_SAMPLE=/tmp/guvenpolice-pdf/sonra.pdf go test ./internal/policypdf -run TestRenderTurkishGlyphs -count=1`. Glif düzeltmesinden önce "ÇGIOŞU çgıoşu", "Urun", "ozeti"; sonra "ÇĞİÖŞÜ çğıöşü", "Ürün", "İlk", "İptal" ve "-1.234,56 TL".
- Canlı belge: müşterinin 100125 numaralı poliçesi. "GüvenPoliçe, Üretim, Ayşe Demir, Ürün, Sürüm aralığı, İlk yazılan prim, İptal iadesi, Cam kırılması" doğru; taksit, faktör dökümü ve TCKN bölümü yok.

## Komutlar ve sonuçlar

| Komut | Sonuç | Süre |
|---|---|---|
| `make test lint build` | PASS | 19 sn |
| `make test-integration` | PASS (`internal/app` 96,1 sn) | 102 sn |
| `npm ci --no-audit --no-fund && npx tsc -b && npm run lint && npm run build` | PASS | 10 sn |
| `docker compose up -d --no-deps --build api`, ardından `web` | PASS; db konteyneri yeniden oluşturulmadı | 24 / 8 sn |
| curl, müşteri kendi poliçesi | 200, `application/pdf`, `attachment; filename="police-100125.pdf"`, `private, no-store`, `%PDF-1.7` | 0,019 sn |
| curl, underwriter / eksper / kimliksiz / müşteri → başka müşterinin poliçesi (50) | 200 / 403 / 401 / 404 | — |
| `make verify` (verify.sh değişmedi; PDF verify'da yok) | 64 PASS / 0 FAIL | 101 sn |
