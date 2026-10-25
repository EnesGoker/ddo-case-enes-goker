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
| F-3, T-4 | PASS (A5; aşağıdaki kanıtlar) |
| F-7, T-5 | A6 uygulandı; aşağıdaki A6 kanıtları |

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

**Bilinen sınırlar:** Uzlaşma iptali yok; APPROVED geri alınamaz. Ayrı olay limiti yok (ADR-10 varsayımı). Bu A4 kontrol noktasından sonra taksit/prim muhasebesi A5, fraud/SLA/atama A6 ile eklendi. A6, idempotans yanıtını bytea ve iş anahtarıyla güçlendirdi (ADR-14). UI/API testleri kurgusal demo veriyle yapılır.

## F-3 / T-4 — Taksit, tahsilat, faiz, iade ve defter (A5)

**Arayüz:** Acente → yeni teklif / teklif detayı → 1–12 taksit seç → poliçeye çevir. Poliçeler → detay → **Ödeme planı**: vadeler, gecikme günleri, tutar/ödenen/durum, faizlerin aralık/anapara/oranı, tahsilat/iadelerin tahsisleri, açık bakiye. Müşteri kendi poliçesinde **Öde**; muhasebe **Ödeme al** (tutar kuruş, yöntem, tarih), ödeme satırını aç → **İade et** (tutar/gerekçe). **Tahsilat ve Defter** → gün sonu tarihi → çalıştır → mizan, 1200 mutabakatı ve satırlarıyla keyset yevmiye listesi. Boş/yükleniyor/hata durumları gerçek API'ye dayanır.

**Seed:** `GP-DEMO-OVERDUE-A` (Gecikmiş-A, ilk seed günü −75 gün), `GP-DEMO-OVERDUE-B` (Gecikmiş-B, −20 gün), üçer taksit, 300.000 kuruş kurgusal prim, başlangıçta ödenmemiş. `policy.Issue` ile plan/limit/yevmiye aynı transaction'da oluşturulur. Sabit poliçe no tekrar seed'de yeni kayıt veya tarih değişikliği yaratmaz. İlk gün: B'nin ilk taksiti 100.000 × 1000 × 20 / 1.000.000 = **2.000 kuruş** faiz; A >=30 gün nedeniyle `NON_PAYMENT` iptal. Tarih ilerledikçe B de 30 gün kuralına girebilir; yeni demo başlangıcı gerekiyorsa yalnızca silinebilir demo ortamında kullanıcı `make reset-demo` çalıştırır. Mevcut veri bu görevde sıfırlanmadı.

**Eski veri:** A5 öncesi plansız poliçede `/billing`, zeyil/iptal ve önizleme 409 `BILLING_PLAN_MISSING`. Otomatik backfill yok; veriyi korumak gerekiyorsa reset yapılmaz. 00005 yalnızca `installment_plans.total_kurus >= 0`; boş veritabanında Up/Down/Up denenir. Sıfır toplamlı plan varken Down, `>0` kısıtı nedeniyle başarısız olabilir; migration yorumu bunu açıklar.

```bash
make test
(cd backend && go test -count=1 -cover ./internal/billing ./internal/caldate)
./scripts/test-integration.sh -coverpkg=./internal/payment/...,./internal/billing,./internal/ledger -coverprofile=/tmp/guvenpolice-a5-coverage.out
(cd backend && go tool cover -func=/tmp/guvenpolice-a5-coverage.out)
./scripts/test-integration.sh -run TestConcurrentPayments -count=10 -race
make lint
make build
make up
make verify
```

- `TestGoldenSplit/Interest/Allocation/Adjustments`, `TestThirtyDayBoundary`: kalan ilk taksite, 31 Ocak → Şubat ay sonu, yarım kuruş, faiz→anapara, çok taksite yayılan ödeme, en geç vadeden azalış/nakit iade, ek taksit, 29/30 gün altın örnekleri.
- `TestBillingIssueThreeInstallments`: API convert üç taksit, prim toplamı/vadeler/tek POLICY_ISSUE yevmiyesi.
- `TestBillingInterestAllocationRefundAndDates`: 100.000/3 → 33.334/33.333/33.333; 20 gün faiz **667**; 40.000 ödeme = 667 faiz + 33.334 ilk anapara + 5.999 ikinci anapara. 1.000 iade ikinciyi 4.999'a indirir. Yalnızca faize tahsis edilen 100 kuruştan 50 kuruş iade, faiz paid alanını 50'ye geri açar. Son tahakkuktan önce ödeme tarihi 422.
- `TestBillingPaymentIdempotency`: aynı anahtarda beş eşzamanlı istek, işlem sürerken mevcut middleware 409 dönebilir; tamamlanmış tekrar 201 ve aynı yanıt, tek PAYMENT/yevmiye. Farklı gövde için anahtar yeniden kullanılmaz.
- `TestConcurrentPayments`: 30.000 açık bakiyeye bağımsız iki HTTP ×20.000. Dış transaction taksit satırını kilitler; `pg_stat_activity` iki isteğin kilitte beklediğini doğrular. Normal sonuç **201 + 422 OVERPAYMENT**, kalan 10.000. `-count=10 -race` PASS.
- **Mutasyon:** yalnızca payment servisindeki poliçe kilidi geçici kaldırıldı; sonuç **201 + 409 CONCURRENT_PAYMENT**. `installments` CHECK ikinci savunması paid>amount taşmasını reddetti; beklenen 201/422 bozulduğu için test FAIL. Kaynak `finally` ile geri konuldu, sonra 10 tekrar `-race` yeniden PASS. Mutasyon kalıcı değil.
- `TestConcurrentRefunds`: aynı ödeme için toplamı iade edilebilir tutarı aşan iki bağımsız eşzamanlı iade → 201 + 422, tek REFUND.
- `TestBillingRunSeedAndBoundary`: seed A iptal, B 2.000 faiz/ACTIVE, aynı gün tekrarında yeni faiz/iptal yok, 29 gün ACTIVE/30 gün CANCELLED, ileri tarih 422.
- `TestBillingRunSkipsLockedAndClaimGuard`: SKIP LOCKED atlama ve A4 hasar korumasıyla `CLAIMS_AFTER_EFFECTIVE_DATE` raporu.
- `TestBillingPremiumAdjustmentsAndCashRefund`: + zeyil ek taksit, − zeyil, peşin ödemenin başlangıç iptalinde nakit REFUND (tahsis yok), manuel iptal sonrası iade 409, ödenmemiş başlangıç iptalinde sıfır plan toplamı.
- `TestRefundReopenedPrincipalAccruesFromRefundDay`: vade gününde tam ödeme, 20 gün sonra 1.000 iade; geçmişe faiz yok, ertesi gün **1 kuruş**, faiz önce kapanır.
- `TestCancelledPolicyAcceptsRemainingBalance`: iptal tarihine kadar kırpılan faiz ve kalan bakiyenin tahsilatı.
- `TestBillingRBACAndLedger`: acente ödeme 403, başkasına müşteri ödeme 404, müşteri iade/gün sonu 403, müşteri kendi ödeme 201, mizan ve satırlı keyset yevmiye.

Her A5 mali testinin sonunda ortak `assertBillingInvariant`: genel defter dengeli, tüm yevmiyeler satırlı/dengeli, **1200 = açık anapara + faiz**, plan toplamları iptal olmayan taksit toplamına eşit.

`make verify` gerçek API ile üç taksit/ilk tahsis/tek ödeme/idem, 422 fazla ödeme, manuel iade, **arka planda iki curl + wait** ile 201/422 yarışı, gün sonu ve A'nın iptalini doğrular. B'deki **her faiz kaydını**, kendi API anapara/ppm/from/to alanlarından Gregorian gün hesabı ve half-up ile awk'te yeniden hesaplar; toplam tutar karşılaştırmasıyla yetinmez. Son mizan `balanced=true`, `matches=true`, satırsız/dengesiz sayıları 0.

## A5 sonuçları (25 Ekim 2026)

| Kontrol | Gerçek sonuç |
|---|---|
| `make test` | PASS; 58 birim test fonksiyonu |
| Saf kapsam komutu (yukarıda) | billing **%92,1**, caldate **%100** |
| Entegrasyon + kapsam komutu | PASS; **52 entegrasyon test fonksiyonu**, A1–A4 dahil, migration 00005 Up/Down/Up |
| Birleşik kapsam profili | payment/store, payment, billing ve ledger toplam **%83,3** |
| `./scripts/test-integration.sh -run TestConcurrentPayments -count=10 -race` | PASS; mutasyon geri alındıktan sonra tekrar 10 kez PASS |
| Poliçe kilidi mutasyonu | Beklenen FAIL; CHECK → 409 CONCURRENT_PAYMENT, fazla tahsilat yok |
| `make lint`, `make build` | PASS; Go vet normal/integration, ESLint, Go/TypeScript/Vite |
| `make up` | PASS; db/api/web healthy; 00005 ve iki demo poliçesi yüklendi |
| `make verify` | **47 PASS, 0 FAIL**, iki ayrı çalıştırma; F-3/T-4 gerçek mali etki ve mutabakat dahil |
| Tarayıcı / 375 px | PASS; acente yeni teklifi 3 taksitle çevirdi (493828/493826/493826 kuruş); muhasebe 100 tahsil/40 iade; müşteri 1 kuruş ödeme ve fazla ödeme hatası; genişlik 375/375, taşma yok |
| OpenAPI | YAML ayrıştırması ve **245 iç `$ref`** PASS; dış linter çalıştırılmadı |

**Sınırlar:** Kurgusal model/oran; kazanılmamış prim karşılığı ve banka entegrasyonu yok. İptal sonrası manuel iade ve otomatik zamanlayıcı yok. Plansız eski veriye backfill yok. Mevcut idempotans middleware'inin iş commit'i–yanıt saklama aralığında süreç çökmesi sınırlaması sürer. Dış OpenAPI linter **ÇALIŞTIRILMADI** (yeni araç kurulmadı); YAML ve iç `$ref` kontrolü yerel Ruby ile yapılır.

OpenAPI yerel kontrolü (repo kökünden):

```bash
ruby <<'RUBY'
require 'yaml'
doc = YAML.load_file('openapi.yaml')
refs = []
walk = lambda do |v|
  case v
  when Hash
    v.each { |k, x| k == '$ref' ? refs << x : walk.call(x) }
  when Array
    v.each { |x| walk.call(x) }
  end
end
walk.call(doc)
refs.each do |r|
  raise "Geçersiz referans: #{r}" unless r.start_with?('#/') && r[2..].split('/').reduce(doc) { |v, k| v.fetch(k) }
end
doc['paths'].each_value do |path|
  path.each_value do |operation|
    next unless operation.is_a?(Hash) && operation['responses']
    operation['responses'].each_value do |response|
      raise "Geçersiz response alanı: #{response}" unless (response.keys - %w[description content headers links $ref]).empty?
    end
  end
end
puts "YAML PASS; #{refs.size} refs PASS; response keys PASS"
RUBY
```

## A6 — Düzeltmeler, T-5 ve F-7

Tüm tutar/risk oranları, fraud puanları, demo kişi ve atölyeler **kurgusaldır**. Altın veri testindeki 2025 tarihleri sabittir; demo FP-1/FP-2 tarihleri **ilk seed gününe göre** seçilir ve sonraki açılışta yerinde güncellenmez. Eski demo poliçe sürümleri ve mali kayıtları korunur.

**Demo hesapları:** Önceki altı hesaba `eksper2@guvenpolice.test` ve `hasar2@guvenpolice.test` eklendi; hepsi `.env` DEMO_PASSWORD değerini kullanır. Atölyeler Kurgusal Marmara/Başkent/Ege (34/06/35). Üç pure paket adı fraud, workflow, lossratio; DB/HTTP task ve report'tur.

**Seed:** `GP-DEMO-FP-1`, 34FRD123, başlangıç ilk gün−300; ilk gün−250 ve −40 tarihli iki hasar skor 0. İkinci dosya, saati ilk gün−4 olan servisle APPRAISAL'a girer (72 saat vade geçmiş). FP-1 müşteri talebiyle ilk gün−6 etkili iptal edilir. `GP-DEMO-FP-2` aynı plaka, başlangıç ilk gün−5, hasar ilk gün−2: EARLY_CLAIM 40 + REPEAT_PLATE 35 =75; FRAUD_REVIEW ve atanmış görev. İki poliçe de başlangıç tarihli tam ödenir; faiz doğmaz. Primler gerçek tarife motorundan gelir. Tekrar seed, iş anahtarları sayesinde yeni hasar/sinyal/görev/yevmiye oluşturmaz.

**Arayüz denemeleri:** Hasar uzmanı → Görevler (tip/durum/yalnız SLA ihlali), Fraud kuyruğu, dosya detayı (75/60, iki açıklama, görev, sunucudan allowed_transitions); red/fraud inceleme notu zorunlu. Eksper → Görevlerim (sunucuda kendi ataması). Muhasebe/underwriter → Raporlar, dahil başlangıç/hariç bitiş, ay/ürün/acente, toplam ve yüzde oran. Alt not yazılan prim/kazanılmış prim sınırlamasını belirtir. 375 px görev listesi ve rapor ekranı ayrıca kontrol edilir; yükleniyor, boş ve hata durumları gerçek isteklere dayanır.

**Kabul testleri ve komutlar:**

| Komut / test | Kanıt |
|---|---|
| `make test` | Sıfır TransitionOptions, SLA sınırları, fraud altın değerleri, oran/taşma, env doğrulaması dahil gerçek birim testleri |
| `./scripts/test-integration.sh` | A1–A6, gerçek geçici PostgreSQL 16; 00006–00008 Up/Down/Up dahil |
| `./scripts/test-integration.sh -run TestConcurrent -count=10 -race` | 0. bölüm kapısında mevcut sekiz yarış testi PASS |
| `./scripts/test-integration.sh -run 'TestConcurrentTaskAssignment\|TestFraudSeedStable' -count=10 -race -v` | 20 bağımsız transaction rol kapısında, üç yeni aktif uzman; her tekrarda 7/7/6. Seed sabitliği ve kapalı DB'de metrics 200 testi de 10 tekrar PASS (296,007 sn) |
| `go test ./internal/fraud ./internal/workflow ./internal/lossratio -coverprofile=/tmp/gp-a6-pure.cover` (backend dizininde) | Üçünde de %100 statement coverage |
| `TestFraudRulesPersistenceAndAutomaticWorkflow` | Her kural tek başına, 75 ve 65 birleşimleri; kalıcı sinyal detayları, NULL otomatik geçmiş aktörü, audit.New.auto, atanmış görev |
| `TestFraudPlateWindowInclusive365` | 365 dahil, 366 hariç; önceki dosya numaraları |
| `TestTaskLifecycleSLAAndRBAC` / `TestTaskEmptyPoolIsUnassigned` | Appraise DONE, çıkışta DONE/redde CANCELLED, boş havuz NULL; injected saat due/due+1µs; ihlal filtresi, sayısal gauge/özet; rol sınırları |
| `TestFraudSeedStableAndMetricsDatabaseFailure` | Tekrar seed değişmeyen mali/sinyal/görev sayısı, FP-1/FP-2 0/0/75; kapalı DB'de 200 ve gauge örnekleri yok |
| `TestBusinessRequestKeyRecovery` | Beş yazma yolunda stale IN_PROGRESS; normal bayt eşitliği; farklı gövde 422; aynı kaynak/yan etki sayıları; işaretli hasarda fraud ve görev tekrarlanmıyor |
| `TestFraudReviewCanReturnToAppraisal` | Ekspertizsiz değerlendirme 409; ekspertizden onay/ödemeye devam |
| `TestProductionReportGoldenLedgerPeriodsAndGroups` | İki ürün, iki acente + acentesiz; +240838/−353582 zeyil ve −18301 iptal, 50000 tazminat; Temmuz 1371738, Ağustos −341883, net1029855 kuruş; [from,to), boyut/defter eşitliği |
| `TestProductionReportPeriodValidationAndRBAC` | Bozuk/eşit/ters dönem 422 INVALID_PERIOD; accounting/UW 200, diğer roller 403 |
| `make lint`, `make build`, `make up`, `make verify` | Derleme/vet/ESLint ve Docker sağlık; verify gerçek curl/DB etkileriyle 59 PASS / 0 FAIL |

**Mutasyonlar commit edilmez.** İade testinde poliçe kilidi kaldırılınca 201+409 CONCURRENT_PAYMENT görüldü; installments CHECK paid>=0 fazla iadeyi engelledi, test beklenen FAIL verdi. Atama advisory kilidi kaldırılınca kapıda 20 bekleyen gözlenmedi ve dağılım 5/7/8 oldu; 7/7/6 testi beklenen FAIL verdi. Her iki dosya aynı baytlarla geri yüklendi:

```text
$ git diff -- backend/internal/payment/service.go
(çıktı yok)
$ git diff -- backend/internal/task/store.go
(çıktı yok)
$ wc -c /tmp/gp-a6-refund-restored.diff /tmp/gp-a6-assignment-restored.diff
0 /tmp/gp-a6-refund-restored.diff
0 /tmp/gp-a6-assignment-restored.diff
0 total
```

**Verify yeni satırları:** F-7 prim-delta, defter-mutabakati, boyut-toplami, oran; T-5 seed-supheli, esik, atama, paralel-atama (farklı poliçeler, arka plan curl + wait), inceleme, sla, yetki; T-4 idem-bayt (cmp). Her çalıştırmada yeni geçerli plaka kullanılır; fraud eşiği susturulmaz. Gün sonu seed B poliçe durumundan bağımsız her faiz kaydını anapara/oran/aralıkla denetler.

**Sınırlar:** ADR-14…18. Önceden tahakkuk etmiş gelecekteki faiz, geriye dönük iptalle iade edilmez. Fraud yeniden skorlanmaz; farklı poliçelerde eşzamanlı sayım READ COMMITTED ±1 danışma sinyali olabilir. Mesai takvimi/manuel atama yok. Kazanılmış prim, muallak, IBNR yok. Gün sonunun büyük veri ölçeği ve hatada devamı A7'ye, genel okunabilirlik turu A8'e aittir. audit.go ve 00001–00005 değişmedi.

Son kaynak ölçümü: **66 birim**, **67 entegrasyon** test fonksiyonu (TestMain ve alt testler dahil edilmedi). İlk son koşuda mevcut auth imza bozma testi Base64'ün kullanılmayan bitleri yüzünden FAIL verdi. Kullanıcı ayrı onay verdi; yalnız test imzanın ilk karakterini bozacak şekilde düzeltildi. Üretim auth kodu değişmedi. `go test ./internal/auth -run '^TestTokenTamperedSignature$' -count=1000` (backend dizininde): PASS.

Atama/seed yarış komutunun tam kopyalanabilir biçimi:

```sh
./scripts/test-integration.sh -run 'TestConcurrentTaskAssignment|TestFraudSeedStable' -count=10 -race -v
```

375×812 tarayıcı kontrolü: görev/ihlal listesi, boş fraud filtresi, 75/60 ve iki açıklamalı dosya, eksperin yalnız kendi görevi, rapor ay/acente gruplaması ve toplam, boş dönem, eşit tarihlerde hata. Görev ve raporda `document.documentElement.scrollWidth === innerWidth === 375` ölçüldü. Son %25 sarı rozet koşulu kaynakta bulunur; **ÇALIŞTIRILMADI:** ayrıca gerçek veriyle sarı rozet senaryosu. Görsel kanıtlar yerel geçici dosyalardır: `/tmp/gp-a6-ui-tasks-375.png`, `/tmp/gp-a6-ui-report-375.png`, `/tmp/gp-a6-ui-appraiser-375.png`. Görünüm boyutu sıfırlandı ve test sekmesi kapatıldı.

## A7 simülatör kullanımı ve kurtarma

Simülatör kurgusal veri üretir. Varsayılan hedef 100.000 poliçe ve 5 yıllık tarih aralığıdır. `make simulate` API açıkken ayrı compose profiliyle çalışır. Aynı seed ve anchor ile tamamlanan koşu tekrar veri eklemez; worker sayısı parmak izini değiştirmez. Ölçüm ve tekrar için anchor gününü açık verin:

```sh
SIM_ANCHOR_DATE=2026-10-25 SIM_SEED=42 SIM_POLICIES=100000 SIM_YEARS=5 SIM_WORKERS=8 make simulate
```

Simülatör bağlantıları `synchronous_commit=off` kullanır. Bu yalnız simülatör havuzuna aittir; API oturum ayarı değişmez. Bağlamdaki tarihsel saat yalnız seed/simülatör tarafından ayarlanır. Audit zamanları gerçek üretim zamanıdır; içerikleri tarihselleştirmek için UPDATE yapılmaz.

Yarım koşu otomatik devam ettirilmez veya mevcut SIM kayıtlarının üzerine yazılmaz. Kurtarma için önce varsa çalışan batch/canlı simülatörü durdurun; `reset-demo` tüm demo ve simülasyon verisini siler, yeniden seed eder. Ardından **aynı seed ve anchor** ile yeniden üretin:

```sh
docker compose --profile sim stop simulator
make reset-demo
SIM_ANCHOR_DATE=2026-10-25 SIM_SEED=42 SIM_POLICIES=100000 SIM_YEARS=5 SIM_WORKERS=8 make simulate
make verify
```

Canlı mod isteğe bağlıdır: `SIM_LIVE=true make simulate`. Yaklaşık 5 saniyede bir gerçek saatle poliçe/tahsilat üretir; hasar üretmez ve deterministik değildir. Verify öncesinde canlı simülatörü durdurun; `simulation_runs.status=LIVE` olduğu sürece verify mutasyonlarına başlamaz.
