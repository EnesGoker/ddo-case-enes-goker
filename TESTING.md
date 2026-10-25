# TESTING

Yalnızca gerçekten çalıştırılmış kontroller burada "PASS" olarak yazılır.

## Demo hesapları

Parola: `.env` içindeki `DEMO_PASSWORD` (örnek dosyada `Demo-Parola-2026`). Tüm kişi bilgileri kurgudur.
Demo müşteriler: Ayşe Demir (müşteri hesabı, 1988, bölge 34), Mehmet Yılmaz, Zeynep Kaya (29 Şubat 2000), Can Öztürk (2007 → genç sürücü, UW onayı) acenteye bağlı; Ali Çelik acentesiz.

| Rol | E-posta |
|---|---|
| Müşteri | musteri@guvenpolice.test |
| Acente | acente@guvenpolice.test |
| Underwriter | underwriter@guvenpolice.test |
| Ekspertiz uzmanı | eksper@guvenpolice.test |
| Hasar uzmanı | hasar@guvenpolice.test |
| Muhasebe | muhasebe@guvenpolice.test |

## Komutlar

```bash
make test               # Go birim testleri
make test-integration   # geçici PostgreSQL 16 konteyneriyle entegrasyon testleri
make lint               # go vet + eslint
make up && make verify  # ayakta sisteme karşı gerçek HTTP kontrolleri
make reset-demo         # demo verisini seed durumuna döndürür
```

## A1 sonuçları (25 Ekim 2026)

| Kontrol | Sonuç |
|---|---|
| `go test ./...` (14 birim test) | PASS |
| `scripts/test-integration.sh` (12 entegrasyon testi, PostgreSQL 16) | PASS |
| `00001` + `00002` Up → Down → Up (`TestMigrationsUpDownUp`) | PASS |
| `npm run build`, `npm run lint` | PASS |
| `docker compose build --no-cache` + `up --wait`: üç servis healthy | PASS (22 sn build + 15 sn açılış; temel imajlar önbellekteydi) |
| `make verify` | 21 PASS, 0 FAIL |
| `make verify` yanlış `DEMO_PASSWORD` ile | FAIL satırları + çıkış kodu 1 (beklenen) |
| `db` durdurulunca `/health` | 503 `{"db":"down"}`; db başlayınca 200 |
| `make reset-demo` | eklenen kayıt silindi, 4 müşteri / 6 kullanıcı geri geldi, `accounts`/`claim_transitions` korundu |

## Manuel UI senaryoları (A1)

1. `http://localhost:3000` → yanlış parola → "E-posta veya parola hatalı." görünür.
2. Acente ile giriş → menüde Profilim, Müşteriler, Teklifler (A2)… → Müşteriler: 3 kayıt, TCKN maskeli.
3. Sayfayı yenile → oturum refresh cookie ile geri gelir.
4. 375 px genişlik → menü yatay kayar, tablo kart görünümüne geçer, yatay sayfa kaydırması yok.
5. Çıkış → yenileyince giriş ekranı (refresh iptal).
6. Müşteri ile giriş → menüde "Müşteriler" yok; API'den de 403 (verify `RBAC-CUSTOMER-403`).

## F-1 — Teklif ve prim hesaplama

**API ile:** `requests.http` → acente girişi → "Teklif önizleme" → "Teklif kaydet" → "Poliçeye çevir" (aynı `Idempotency-Key` ile ikinci kez).
**Beklenen:** önizleme ve kayıt aynı toplamı/dökümü/`input_hash`'i verir; kayıt 201 ve durum `CALCULATED`; çevirme 201 + `policy_no`; tekrarında aynı gövde ve `Idempotent-Replayed: true`; farklı anahtarla ikinci çevirme 409 `QUOTE_NOT_CONVERTIBLE`.
**Arayüzde:** acente → *Yeni teklif* → Ayşe Demir, Kasko, kademe 2, grup C, plaka, Cam seçili → *Primi hesapla* (döküm tablosu: her faktörün TL etkisi) → *Teklifi kaydet* → *Poliçeye çevir* → yeşil kutuda poliçe no. Underwriter → *Teklifler* (varsayılan filtre "Underwriter onayında") → Can Öztürk teklifi → *Reddet* (notsuz → "red gerekçesi zorunlu") → *Onayla*. Müşteri (Ayşe) → *Teklifler*/*Poliçeler*: yalnızca kendi kayıtları.
**Kanıt komutu:** `make verify` → `F-1 teklif-kaydet-oku`, `F-1 police-cevir`, `F-1 cift-cevirme`, `F-1 uw-onay` satırları.

## T-1 — Prim hesaplama motoru

**Beklenen:** aynı girdi + aynı tarife → aynı döküm, toplam ve hash; döküm satırları toplamı = nihai prim; tarife poliçe başlangıç tarihine göre seçilir (KASKO: 2026-06-30 → v1, 2026-07-01 → v2).
**Kanıt komutları:**

```bash
cd backend && go test -count=1 -v -cover -run 'TestGolden|TestDeterminism|TestBreakdownSumEqualsTotal|TestMulBPHalfUp|TestSelectTariffByStartDate|TestAgeRangeBoundaries' ./internal/pricing/
make verify   # T-1 determinizm, T-1 dokum-toplami, T-1 tarife-tarihi
```

Altın örneklerin elle hesabı `backend/internal/pricing/engine_test.go` içinde adım adım yorum olarak yazılıdır (KASKO 1.892.976 kuruş, KONUT 219.450 kuruş).

## A2 sonuçları (25 Ekim 2026)

| Kontrol | Sonuç |
|---|---|
| `go test ./...` | PASS (34 birim test fonksiyonu) |
| `go test -cover ./internal/pricing ./internal/caldate ./internal/platform/page` | pricing %95,5 · caldate %100 · page %100 |
| `scripts/test-integration.sh` | PASS (23 entegrasyon testi; 11'i A2: determinizm, tarife tarihi, doğrulama, UW akışı, convert idempotansı, farklı anahtarlarla eşzamanlı 5 convert → 1 poliçe, aynı anahtarla eşzamanlı 5 istek → 1 poliçe, süresi geçmiş IN_PROGRESS devralma, RBAC/sahiplik, poliçe listesi kapsamı) |
| Mutasyon denemesi: convert'teki `FOR UPDATE` kaldırıldı | `TestConcurrentConvertDifferentKeys` FAIL (kaybedenler 409 yerine 500) → yarış testi kilidi gerçekten ölçüyor; kilit geri konuldu |
| `npm run build && npm run lint` | PASS |
| `make verify` | 28 PASS, 0 FAIL (F-1 ×4, T-1 ×3 dahil) |
| Tarayıcı (acente, underwriter, müşteri) | yukarıdaki arayüz senaryosu çalıştı; 375 px'de yatay taşma yok |

## F-2 — Poliçe düzenleme, zeyil, iptal

**API ile:** `requests.http` → poliçe detayı → zeyil önizleme → zeyil (Idempotency-Key) → tarihte teminat (zeyilden bir gün önce / zeyil günü) → iptal önizleme → iptal.
**Beklenen:** zeyil 201, yeni sürüm zeyil tarihinde başlar, eski sürüm aynı gün kapanır; aynı anahtarla tekrar → aynı zeyil (`Idempotent-Replayed: true`); iptal 201, poliçe `CANCELLED`, açık sürüm iptal tarihinde kapanır; iptal gününde ve sonrasında `coverage` → 404 `NO_COVERAGE_ON_DATE`; iptal edilmiş poliçeye zeyil → 422 `POLICY_CANCELLED`.
**Arayüzde:** acente → *Poliçeler* → poliçe satırı → detay (sürüm geçmişi, güncel teminatlar, prim özeti) → *Araç değişikliği* (grup D) → *Önizle* (gün sayıları, hesap adımları, yeni döküm) → *Zeyli onayla* → sürüm v2 ve zeyil #1 listelenir → *Tarihte geçerli teminat*: başlangıç günü v1, zeyil günü v2 → *İptal et* (acentede yalnızca müşteri talebi) → *Önizle* (iade ve adımlar) → *İptali onayla* → durum "İptal", net prim = kazanılmış prim. Müşteri (Ayşe) aynı detayı işlem butonları olmadan görür.
**Kanıt komutu:** `make verify` → `F-2 zeyil-kayit`, `F-2 surum-gecmisi`, `F-2 iptal-sonrasi`.

## T-2 — Zeyil ve gün esaslı iade

**Beklenen:** zeyil farkı = (yeni − eski yıllık) × kalan / dönem (kuruşa, yarımı yukarı; negatifte sıfırdan uzağa); dönem içinde 29 Şubat varsa 366 gün; müşteri talebiyle iptal kısa dönem tablosuyla, şirket kaynaklı iptal gün esaslı.
**Kanıt komutları:**

```bash
cd backend && go test -count=1 -v -cover ./internal/proration/
make verify   # T-2 zeyil-gun-esasli, T-2 tarihte-teminat, T-2 kisa-donem-iade
```

Altın örneklerin elle hesabı `backend/internal/proration/proration_test.go` içinde adım adım yorum olarak yazılıdır (araç değişikliği +199.670 kuruş, kısa dönem iptal iadesi 250.150 kuruş, aynı tarihte gün esaslı iade 721.924 kuruş, 29 Şubat tarihli zeyil 93.000 kuruş).

## A3 sonuçları (25 Ekim 2026)

| Kontrol | Sonuç |
|---|---|
| `go test ./...` | PASS (49 birim test fonksiyonu; 15'i A3: proration 14 + `db.Violates` 1) |
| `go test -cover ./internal/proration` | %97,8 (pricing %95,5 · caldate %100 · page %100) |
| `scripts/test-integration.sh` | PASS (32 entegrasyon testi; 9'u A3: değişmezlik tetikleyicisi, zeyil + tarihte teminat + idempotans, kısa dönem iptal, başlangıç gününde gün esaslı iptal + RBAC, doğrulamalar, limit < kullanım, eşzamanlı 5 zeyil, eşzamanlı zeyil + iptal ×4, 23505 → 409) |
| `00001`+`00002`+`00003` Up → Down → Up; 00003 tek başına Down/Up | PASS (`TestMigrationsUpDownUp`) |
| Mutasyon 1: convert'teki `FOR UPDATE` kaldırıldı (5 tekrar) | Eşzamanlı convert testi PASS: kaybedenler artık 500 değil 409 (A3-0 düzeltmesi); kilit geri konuldu |
| Mutasyon 2: poliçe satırı kilidi (`FOR UPDATE OF p`) kaldırıldı (3 tekrar) | `TestConcurrentEndorsements` ve `TestConcurrentEndorsementAndCancellation` FAIL (kaybedenler 201/422 yerine 409 `CONCURRENT_MODIFICATION`) → testler kilidi ölçüyor; koşullu UPDATE'ler sayesinde veri yine tutarlı kaldı; kilit geri konuldu |
| `npm run build && npm run lint` | PASS |
| `make verify` | 34 PASS, 0 FAIL (F-1 ×4, T-1 ×3, F-2 ×3, T-2 ×3) |
| Tarayıcı (acente, müşteri) | zeyil önizleme/onay, tarihte teminat (v1/v2), kısa dönem iptal önizleme/onay, müşteri salt okunur; 375 px'de yatay taşma yok |

## F/T kimlikleri

| Kimlik | Durum |
|---|---|
| F-1 | PASS (`make verify`) |
| T-1 | PASS (`make verify` + birim/entegrasyon testleri) |
| F-2 | PASS (`make verify` + entegrasyon testleri) |
| T-2 | PASS (`make verify` + birim/entegrasyon testleri) |
| F-4, F-5, F-6, T-3 | PASS (A4; aşağıdaki kanıtlar) |
| F-3, F-7, T-4, T-5 | YAPILMADI (A5 ve sonrası) |

## F-4 / F-5 / F-6 — Hasar, ekspertiz ve tazminat (A4)

1. Müşteri/acente: bugün başlayan bir poliçeyi aç → **Hasar bildir** → geçmiş/bugünkü hasar tarihi ve o tarihte geçerli teminatı seç. Dosya REPORTED, geçmiş `NULL → REPORTED`; sürüm/plaka sunucudan gelir.
2. Hasar uzmanı: **Hasar Dosyaları** → dosya → Ön incelemede (TRIAGED) → Ekspertizde (APPRAISAL). Her geçiş güncel `expected_row_version` gönderir. Red için gerekçe zorunludur.
3. Eksper: **Ekspertiz** varsayılan APPRAISAL listesi → brüt hasar, inceleme zamanı, pert durumu ve not → kaydet. Birden çok kayıtta son eklenen ekspertiz kullanılır.
4. Hasar uzmanı: ADJUSTMENT → **Tazminat önizle** → altı adımı ve ödenecek tutarı incele → **Tazminatı onayla**. Örnek CAM: 2.000.000 − 50.000 sabit muafiyet = 1.950.000 kuruş (kalan limit yeterliyse).
5. Muhasebe: **Hasar Dosyaları** varsayılan APPROVED listesi → dosya → **Öde**. PAID, `paid_at` ve `payment_journal_entry_id` görünür. Müşteri/acente kendi dosyasını; underwriter tüm dosyaları salt okunur izler.

**Kanıt komutu:** `make verify` → `F-4 hasar-dosya`, `F-5 ekspertiz`, `F-6 idempotent-onay`, `F-6 odeme`. `TestClaimLifecycleAndIdempotency`: aynı anahtarla onay/ödeme tekrarı aynı yanıt, tek settlement ve olay başına tek yevmiye; her yevmiye ve tüm defter borç = alacak. `TestClaimValidationAndLedgerRollback`: bildirim tekrarında aynı kayıt, istemciden sürüm kabul edilmemesi, eksik tutar/atölye doğrulaması ve eksik muhasebe hesabında bütün yevmiyenin geri alınması.

## T-3 — Tarihte sürüm, muafiyet ve eşzamanlı limit (A4)

```bash
(cd backend && go test -count=1 -v -cover ./internal/settlement)
./scripts/test-integration.sh -run TestConcurrentClaimApprovals -count=10
./scripts/test-integration.sh -run TestConcurrentClaimApprovals -count=10 -race
./scripts/test-integration.sh -coverpkg=./internal/claim,./internal/ledger,./internal/settlement -coverprofile=/tmp/guvenpolice-a4-coverage.out
(cd backend && go tool cover -func=/tmp/guvenpolice-a4-coverage.out)
make up
make verify
```

Komutlar repo kökünden çalıştırılır; parantezli komutlar kendi alt kabuğunda çalışır. Entegrasyon betiği geçici PostgreSQL 16 konteyneri ve her çalıştırmada geçici veritabanı kullanır; gerçek demo verisini silmez.

- `TestGoldenSettlements`: elle hesap yorumlu sabit muafiyet, yüzde + yarım kuruş, teminat ve yıllık limit kırpması; ayrıca muafiyet > brüt, NONE, MaxInt64. Saf paket kapsamı **%100**.
- `TestClaimVersionSelectionAndCancellation`: zeyilden bir gün önce eski sürüm/plaka, zeyil günü yeni sürüm/plaka; iptal tarihinde/sonrasında, başlangıç gününde iptal ve poliçe bitişinde 422.
- `TestClaimsBlockBackdatedPolicyChanges`: hasar tarihine eşit/önceki zeyil ve iptal, önizleme dahil `422 CLAIMS_AFTER_EFFECTIVE_DATE`; REJECTED hasar engel oluşturmaz.
- `TestClaimTransitionsValidationAndRBAC`: geçersiz geçiş 409, eski sürüm 409, gerekçesiz red 422; APPRAISER onaylayamaz, CLAIMS_ADJUSTER ödeyemez, ACCOUNTING bildirim yapamaz, CUSTOMER başkasına hasar açamaz (404). Eksper kapsamı, son ekspertiz, durum filtresi/keyset de doğrulanır.
- `TestConcurrentClaimApprovals`: yıllık 1.000.000, teminat 2.000.000; iki ayrı dosya × 800.000. İstekler dış transaction kilidinde buluşur; iki bağımsız HTTP bağlantısı/transaction onaylar → **800.000 + 200.000**, yıllık kullanım **1.000.000**. `-count=10` ve `-count=10 -race` PASS.
- **Mutasyon:** sadece bakiye sorgusunun `FOR UPDATE` eki geçici kaldırıldı. Aynı test ikinci onayda `409 LIMIT_CHANGED` ile FAIL oldu; koşullu UPDATE ikinci savunmada taşmayı engelledi (kullanım 800.000). Kilit geri konuldu; 10 tekrar `-race` PASS. Mutasyon kalıcı kodda yoktur.
- `make verify` gerçek katalogda iki farklı teminatı paralel onaylar: KASKO_ANA brüt 150.000.000 (%2 muafiyet sonrası 147.000.000), IMM 50.000.000; toplam istek 197.000.000 → ortak yıllık bakiye **180.000.000** ile kırpılır. Sıraya göre 147.000.000 + 33.000.000 veya 130.000.000 + 50.000.000. Entegrasyon fixture'ında ise aynı teminatta iki kez yıllık limitin %80'i denenir.

## A4 sonuçları (25 Ekim 2026)

| Kontrol | Gerçek sonuç |
|---|---|
| `make test` | PASS; 52 birim test fonksiyonu (alt testler hariç) |
| `./scripts/test-integration.sh -coverpkg=./internal/claim,./internal/ledger,./internal/settlement -coverprofile=/tmp/guvenpolice-a4-coverage.out` | PASS; 40 entegrasyon test fonksiyonu; migration Up/Down/Up (00004 dahil) |
| Birleşik kapsam profili (`go tool cover -func`) | claim %79,9; ledger %90,9; settlement %100; bu üç paket toplam %81,5 |
| `./scripts/test-integration.sh -run TestConcurrentClaimApprovals -count=10 -race` | PASS; gerçek paralel onay, 10 tekrar, yarış algılayıcısı |
| Bakiye kilidi mutasyonu | Beklenen FAIL; koşullu UPDATE `409 LIMIT_CHANGED`; kilit geri konuldu |
| `make lint` | PASS; normal/integration `go vet` + ESLint |
| `make build` | PASS; Go + TypeScript/Vite |
| `make up` | PASS; db/api/web healthy, yeni 00004 migrasyonu uygulandı |
| `make verify` | 40 PASS, 0 FAIL; A1–A3 kontrolleri de korunuyor |
| Tarayıcı / 375 px | PASS; altı rol menü/liste; müşteri bildirimi → ekspertiz → onay → ödeme; hata/boş durumları; içerik genişliği 375 px, yatay taşma yok |
| OpenAPI | Ruby YAML ayrıştırması ve 202 iç `$ref` doğrulaması PASS; dış OpenAPI linter ÇALIŞTIRILMADI (yeni araç kurulmadı) |

**Bilinen sınırlar:** Uzlaşma iptali yok; APPROVED geri alınamaz. Ayrı olay limiti yok (ADR-10 varsayımı). Fraud/SLA/atama A6, taksit/prim muhasebesi A5 kapsamındadır. İdempotans JSONB yanıt sırası ve iş transaction'ı–yanıt saklama aralığı A2/A3'teki gibi kalır. UI/API testleri kurgusal demo veriyle yapılır.
