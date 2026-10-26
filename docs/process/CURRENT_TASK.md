# B2 — Poliçe PDF çıktısı

**Durum: TAMAMLANDI.** Taban `ec70df6`; `bonus/police-pdf` dalında geliştirildi ve main'e alındı (`d01618a`).

## Kapsam

- `GET /api/v1/policies/{id}/pdf`: poliçe detay ucunun o role döndürdüğü verinin aynısı, "son kayıtlı sürüm" etiketiyle. Taksit planı, prim faktör dökümü ve TCKN yok.
- Yetki detay ucuyla aynı: kimliksiz 401, eksper 403, kapsam dışı veya olmayan kayıt 404.
- Tek read-only repeatable-read transaction; açık transaction'da havuzdan ikinci bağlantı yok; PDF transaction kapandıktan sonra çizilir.
- Yanıt `application/pdf`, `attachment; filename="police-{id}.pdf"`, `Cache-Control: private, no-store`; hatalar JSON hata biçiminde.
- signintech/gopdf v0.38.3 (MIT), NotoSans-Regular.ttf ve OFL 1.1 (`go:embed`); para int64 kuruştan Türkçe TL.
- Frontend: poliçe detayında "PDF indir"; mevcut oturum ve refresh akışıyla blob indirme.

## Sonuç

- Görsel kontrolde bulunan iki hata düzeltildi: negatif tutarda ASCII "-" (U+2212 fontta yok); iç içe bileşik gliflerde kaybolan Ö/Ü/Ğ/İ işaretleri için gömülü font bellekte düzleştirildi.
- `make test lint build`, `make test-integration` ve CI frontend komutları PASS; `make verify` 64 PASS / 0 FAIL. PDF verify'a eklenmedi; entegrasyon testiyle doğrulanır.
- Değişmeyenler: şema ve migration, Dockerfile, CI, docker-compose, `scripts/verify.sh`, README "Doğrulama Soruları"; db konteyneri ve 100k verisi.

## Commit'ler

- `6ef011f` docs: B2 görev tanımı
- `9908260` feat(policy): yetkili poliçe PDF çıktısını ekle
- `9085640` feat(ui): poliçe PDF indirme işlemini ekle
- `1072b3a` docs: B2 ara kontrol noktasını kaydet
- `90c8bbe` fix(policy): PDF'te negatif tutar ve Türkçe glif sorununu düzelt
- `15e2971` fix(policy): PDF içeriğini detay ucunun verisiyle sınırla
- docs: poliçe PDF bonusunu belgele (bu kayıt)

## Kanıt

Komutlar, süreler ve görsel kontrol: [B2 kabul kaydı](../testing/b2-pdf.md). Karar: [ADR-28](../../DECISIONS.md#adr-28--poliçe-pdf-özeti-b2).
