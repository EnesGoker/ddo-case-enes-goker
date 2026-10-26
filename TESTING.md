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
docker stop guvenpolice-simulator
make reset-demo
SIM_ANCHOR_DATE=2026-10-25 SIM_SEED=42 SIM_POLICIES=100000 SIM_YEARS=5 SIM_WORKERS=8 make simulate
make verify
```

Canlı mod isteğe bağlıdır: `SIM_LIVE=true make simulate`. Yaklaşık 5 saniyede bir gerçek saatle poliçe/tahsilat üretir; hasar üretmez ve deterministik değildir. Canlı mod durduğunda parmak izi temizlenir; gösterilen özet toplu koşunun sonundandır. Verify öncesinde canlı simülatörü durdurun; `simulation_runs.status=LIVE` olduğu sürece verify mutasyonlarına başlamaz.


### A7 gerçek kabul sonuçları

| Kontrol | Gerçek sonuç |
|---|---|
| Gün sonu: üç parça/tek sonuç, imleç, hata rollback, gelecek vade, yalnız faiz, gerçek 8 sn bütçe | PASS |
| API yeni kayıt zamanları gerçek istek aralığında | PASS; API bağlam saati ayarlamaz |
| 00009/00010 Up/Down/Up; mevcut iade indeksiyle tekrar Up; reset sonrası v0 | PASS; taze test DB'leri |
| Kapı: tüm entegrasyon, 9 yarış ×10/race, verify | PASS; kapıda 60 PASS / 0 FAIL |
| Go birim/race, vet, frontend lint/build | PASS |
| İlk 1000 üretimi | FAIL; Docker disk doldu, tamamlanmış ölçüm sayılmaz |
| Aynı seed + 8/16 worker aynı parmak izi, başka seed farklı; mali/izolasyon/geçişler | PASS; üç taze DB |
| Canlı mod / verify ön kontrolü | PASS; gerçek LIVE koşusunda verify çıkış2, iş uçlarına istek yok |
| 100000 / 5 yıl, üretici v1 | PASS; 50 dk25,12 sn; DB 2.217.606.167 bayt (2115 MB) |
| 100k sonrası verify | PASS; 61 PASS / 0 FAIL; 112,11 sn (<15 dk); gün sonu 103 parça |
| Beş yıllık rapor / kuyrukta SIM hasarları | PASS; 2021-10…2026-10 aralığında 61 ay, ilk20 kuyruk satırının18'i SIM |
| Her ADJUSTMENT girişinde ekspertiz | PASS; dört entegrasyon durumu, 409 APPRAISAL_REQUIRED; 375 px UI disabled |
| Sarı SLA kabul verisi | PASS; servislerle ayrı GP-DEMO-SLA-YELLOW / GP-H-00007968; yaklaşık12 saat kalan72 saatlik görev, badge sla-soon |

**100k koşusunun kapsamı:** seed42, anchor2026-10-25, 5 yıl,16 worker. Bu ölçüm eski üretici sürüm1 /30 atölye içindir; paralel test, UI ve çevrimiçi indeks incelemesi vardı, izole benchmark değildir. 100.000 SIM poliçe; Kasko50.000/Trafik35.000/Konut15.000; 7.915 hasar; 1.900 işaretli; açık630/ihlalli621 görev. Özetin defter dengesi ve1200 mutabakatı true. Parmak izi: `abc70939dcba41e974d1648012141bc97015c9bb1075750aee274c6a814d8272`. Özet batch bittiği andaki görüntüdür; sonradan verify/gün sonu ve rol denemeleri ana DB'ye yeni demo işlemleri yazdı.

### İşaretli hasar kırılımı ve üretici v2

Eski100k'da oran **1.900 /7.915 =%24,01**. Kuralların kesişimi:

| İşaretli dosyadaki sinyaller | Dosya | Kasıtlı üçlü örneği |
|---|---:|---:|
| EARLY_CLAIM + WORKSHOP_CONCENTRATION | 1300 |1197 |
| EARLY_CLAIM + REPEAT_PLATE + WORKSHOP_CONCENTRATION |599 |599 |
| EARLY_CLAIM + REPEAT_PLATE |1 |1 |

Kural katılımı EARLY1900 /REPEAT600 /WORKSHOP1899; satırlar kesiştiği için toplanmaz. 1797 kasıtlı,103 normal örnek işaretlidir. Atölye sayısı300 oldu; kasıtlı üçlüler her1000'de ikiye seyreltilirken ilk altı dosyalık yakın dönem patlama korundu. Normal Kasko sıklığı%10,7; saf100k planında yıllık toplam1201bp (Trafik628bp/Konut407bp). Bu saf plan testi DB'de100k üretimi değildir. **Fraud kuralları ve eşikleri değişmedi.**

Aşağıdaki gerçek DB kabulü `./scripts/test-integration.sh -run 'TestSimulatorFraudRateAt1kAnd10k|TestSimulatorDeterminismAndInvariants' -v -timeout 20m` içindeki Go testlerinde geçti:

| V2 örneği | Atölye | Hasar | İşaretli | Oran | Atölye sinyali | Servis üretimi ve son kontrol süresi |
|---|---:|---:|---:|---:|---:|---:|
|1000 poliçe |300 |85 |3 |%3,53 |2 |35,516 sn |
|10000 poliçe |300 |810 |44 |%5,43 |36 |514,073 sn |

Her ikisinde defter dengesi ve1200 mutabakatı PASS. Seed42/anchor2026-10-25/16 worker, taze PostgreSQL16 DB, host→Docker bağlantısı; izole performans ölçümü değildir. Parmak izleri sırasıyla `4a689b6577278eafdfedd3f44ea0463629d9fc8d0acf423bb4e0741f786983c5` ve `92bf44c8ad91587a6b0431fe3f15615886807ce0b39558597a1aa0d97457d75a`. İşaretli dosyalarda EARLY/REPEAT/WORKSHOP katılımları 1k için3/3/2,10k için44/21/36. Ek deterministik1k testleri koşu gününün anchor değerini (2026-10-26) kullandığından farklı parmak izi üretir;8/16 worker sonucu kendi içinde aynı, seed43 farklıdır.

İlk hedefli komutta tüm Go paketleri PASS (app723,399sn), fakat çalışan kabuk betiği o sırada düzenlendiği için sonrasında `ette: command not found` ile127 döndü. Bu komutun tamamına PASS yazılmadı; son kaynakla `make test-integration` tekrarında komut çıkışı0, bütün paketler PASS (app880,689sn; `/tmp/gp-a7-final-integration-v2.log`). Entegrasyon betiğinin varsayılan Go bütçesi10k servis kabulü için20 dakikadır; API zaman bütçeleri değişmedi.

**Kalan sınırlama:** Atölye eşiği mutlak adettir; atölyenin kendi normal hacmine göre anomali ölçmez. 1k/10k oranları yeni100k oranını kanıtlamaz. Kullanıcı talimatıyla yeni100k bu gece **ÇALIŞTIRILMADI**; mevcut sürüm1 veri kümesi korunuyor. Sabah A8 ölçümlerinden önce, yeni imajla:

```sh
make reset-demo
SIM_ANCHOR_DATE=2026-10-25 SIM_SEED=42 SIM_POLICIES=100000 SIM_YEARS=5 SIM_WORKERS=16 make simulate
make verify
```

Reset tüm demo/simülasyon kabul kayıtlarını da siler. İşlem yarıda kalırsa yukarıdaki aynı reset→simulate→verify yolu tekrarlanır. Yeni100k'nın oranı ve performansı gerçek koşudan sonra ayrıca raporlanmalıdır.

### A7 rol kabulü (375×812, gerçek API, simülasyon yüklü)

| Rol | Akış ve gözlenen sonuç | Sonuç |
|---|---|---|
| CUSTOMER | Kendi poliçesi34 →2000TL kısmi ödeme →fazla ödeme hatası →GP-H-00000040 bildirimi/sürüm1/takip; Sistem | PASS |
| AGENT | Kendi müşteri listesi →teklif34/35 →UW sonrası çevirme →zeyil →başlangıçta iptal; Sistem | PASS |
| UNDERWRITER |35 no'lu teklif onayı →SIM poliçede Süresi doldu/tarihsel teminat →ay/ürün raporu →boş dönem ve eşit tarih hatası; Sistem | PASS |
| APPRAISER | Yalnız kendi görevleri →dosya40'a9000TL ekspertiz →boş İncelemede filtresi; Sistem100k özeti | PASS |
| CLAIMS_ADJUSTER | Gerçek SIM fraud kuyruğu/sinyalleri →ekspertiz sonrası önizleme/onay; ekspertizsiz buton pasif; sarı/kırmızı SLA; Sistem | PASS |
| ACCOUNTING | Dosya40'a8820TL ödeme; poliçe47'ye1000TL tahsilat/100TL iade →gün sonu Tamamlandı →dengeli mizan/ürün raporu/Sistem | PASS |

Menü erişimleri gerçek rol oturumlarıyla denendi; listeler20 satırla başlar. Defterde Daha fazla kayıt20 ek satır getirdi. Sistem, görev ve raporda scrollWidth=innerWidth=375; geçici viewport sıfırlandı ve kabul sekmesi kapatıldı. Yükleniyor/boş/hata durumları gerçek API dönüşleriyle gözlendi. Yoğun16-worker100k üretimiyle paralel mizan sorgusu15sn bütçesini aşarak504, canlılık sorgusu bazen3sn'de503 verdi; UI hatayı/yeniden denemeyi gösterdi. Üretim bitince aynı mizan API'si200/2,670sn, defter dengeli ve mutabık. Denenen00011 kaplama indeksi varsayılan planı iyileştirmediği için Down ile geri alındı, kaynak dosyası kaldırıldı; kalıcı indeks yalnız00010'dur. Ağır yazma yükünde mizan gecikmesi bilinen sınırlamadır.

Sarı SLA verisi kullanıcı onayıyla yalnız servislerden, tarihsel seed bağlamıyla üretildi; varsayılan seed'e eklenmedi. API saati değişmedi. Geçici kabul yardımcısı kaynakta kalmadı. Görsel kanıtlar: `/tmp/gp-a7-sla-yellow-375.png`, `/tmp/gp-a7-adjustment-disabled-375.png`, `/tmp/gp-a7-system-100k-375.png`, `/tmp/gp-a7-accounting-balanced-375.png`.

Son v2 imajı derlendi ve API sağlıklı yeniden açıldı. Paralel10k entegrasyon üretimi sırasında yapılan ek `/usr/bin/time -p make verify` **58 PASS /3 FAIL**,279,62sn: T-4 mizan isteği10sn istemci süresini aştı (000); F-7 defter ve boyut kontrolleri de başarısız oldu. Bu koşu PASS sayılmadı. Önceki112,11sn/61PASS100k kabulü ayrı gerçek koşudur. Test yükü bittikten sonraki tekrarın sonucu PROGRESS'te ayrıca yer alır.

Test yükü bittikten sonra aynı v2 imajıyla `/usr/bin/time -p make verify`: **61 PASS /0 FAIL,real110,74sn,exit0** (`/tmp/gp-a7-final-verify-alone.log`). Gün sonu103 parça, her yevmiye dengeli,1200 mutabık, F-7 bütün boyut toplamları aynı ve SYS canlılığı güncel. Bu son doğrulama mevcut v1/100k veri üzerinde yapıldı; v2/100k üretimi değildir.

## A8 düzeltme kabulü (olay limiti öncesi)

Simülasyon testleri `integration && simulation`; `make test-simulation` ayrı hedefidir. `make test-integration` ağır üretimi içermez. Varsayılan açık iş penceresi SIM_OPEN_WINDOW=48h. Taze DB'de seed42/anchor2026-10-25:1k2açık inceleme/2görev/0ihlal,10k7açık inceleme/7görev/1ihlal(%14,29); işaretli oranları%3,53/%4,96. İki örnekte DONE APPRAISAL sahibi ile ekspertiz aktörü uyuşmazlığı0 ve mali mutabakat doğru. Tam simülasyon kabul komutu PROGRESS'te; app643,392sn,exit0. Yeni100k'nın sonucu henüz bu ölçümlerden çıkarılamaz.

V0 başlangıcı2020-01-01;2019-12-31 tarife-yok.00011 Up/Down/Up referans/prim koruması ve2019bağlı kayıt varken rollback testi PASS. Müşteri/acente tüm hasar yanıtlarında UNDER_REVIEW ve gizli iç notlar; red gerekçesi görünür. Görevi kapatan insan atamayı devralır/eski atanan audit'e yazılır. Karar aşaması reddi DONE, iş yapılmadan iptal CANCELLED; SLA tanımı liste/özet arasında aynı. Hedefli8DB kabulü app17,172sn PASS.

### A8 T-3 olay başı limit

00012: policy_version_coverages.event_limit_kurus, NULL=yok/varsa pozitif. Eski kayıtlar yeniden fiyatlanmaz. Kurgusal katalog CAM10.000TL olay/25.000TL dönem;500TL sabit muafiyet. Saf TestGoldenEventLimitsInOrder dört sınırın sırasını; TestEventLimitInt64AndValidation büyük tutar/geçersiz girişi doğrular. Yeni hesapta7adım, NULL olay adımı skipped; geçmiş settlement dökümleri korunur.

TestEventLimitSnapshotAndPerClaimApproval gerçek API'de iki ayrı olayın her birinde1.000.000kuruş onayı, toplam2.000.000kuruş tüketimi, zeyilde snapshot ve eski NULL davranışını doğruladı. TestEventLimitMigrationUpDownUpAndDataGuard PASS. `make test-integration` PASS/exit0 (app70,830sn), lint/build PASS. Yeni verify T-3 olay-limiti satırı son100k sonrasında çalışacak.

## A8 iç inceleme regresyonları

`./scripts/test-integration.sh -run 'TestActivity|TestBusinessKeyRejects|TestUnchangedBilling|TestAppraisalIdempotency|TestRefundIndexRecovers|TestBusinessRequestKeyRecovery|TestMigrationsUpDownUp' -v`: PASS (app13,442sn). Yanlış kind/refund_of/tutar veya zeyil türü422, etkisiz gün sonu audit0, ekspertiz tekrarında1kayıt, INVALID çevrimiçi indeks kurtarma. Tam normal entegrasyon PASS76,972sn; simülasyon testleri ayrı.

Ekspertiz isteğinde Idempotency-Key zorunlu. UI değişmeyen formu tekrar gönderirken aynı anahtarı korur; alan değişince yeniler. Sistem sayfasında tüm roller canlılık sayısını görür; mali simülasyon özeti yalnız muhasebe/UW'ye açıktır. Verify macOS BSD date üzerinde nanosaniye için Python3 gerektirir; GNU date doğrudan kullanılır.

## A8 şartname izlenebilirliği

Opus'un e457cf5 üzerinden çıkardığı salt okunur tablo, A8'de uygulanmış kod/testlerle güncellendi. Eski A7 ölçümleri yeni100k kabulü yerine geçmez. Aşağıdaki PASS otomatik test veya geçmişi belirtilmiş gerçek kabul kanıtına dayanır; henüz ölçülmeyen teslim maddesi BEKLİYOR'dur.

| Şartname maddesi | Kanıt / deneme | Durum |
|---|---|---|
| §2 altı rol, sunucuda RBAC/401/403 | router RequireRoles, TestAllDemoRolesCanLogin, yetki entegrasyonları, verify RBAC | PASS |
| §3 F-1 teklif + prim dökümü | TestQuoteDeterministicAndPersisted, TestGoldenKasko; verify F-1 | PASS |
| F-2 poliçe/zeyil/iptal, geçmiş | TestConvertCreatesPolicyAndIsIdempotent, TestVehicleChangeEndorsementAndCoverageOnDate, TestPolicyVersionsAreImmutable | PASS |
| F-3 plan/kısmi ödeme/faiz/iade | TestBillingInterestAllocationRefundAndDates, TestRefundReopenedPrincipalAccruesFromRefundDay | PASS |
| F-4 bildirim/durum/geçmiş | TestClaimLifecycleAndIdempotency, TestCustomerAndAgentClaimBodiesHideReviewInternals | PASS |
| F-5 gerçek ekspertiz | TestAppraisalIdempotencyAndRequiredKey, TestAdjustmentRequiresAppraisalOnEveryEntry | PASS |
| F-6 hesap/onay/ödeme | TestGoldenSettlements, TestClaimLifecycleAndIdempotency | PASS |
| F-7 prim/hasar oranı raporu | TestProductionReportGoldenLedgerPeriodsAndGroups, dönem/gruplama/RBAC testleri | PASS |
| Gerçek API, yükleniyor/boş/hata | A7 altı rol tarayıcı kabulü; frontend api/client ve States | PASS (A7) |
| Mobil kullanılabilirlik | A7 375×812 altı rol ve sarı SLA kabulü; aşağıdaki A8 tekrar ölçümü ayrı kaydedilir | PASS (A7) |
| T-1 yaş/NCD/bölge/araç faktörleri | TestAgeRangeBoundaries, TestNCDLevels, TestGoldenKasko/Konut | PASS |
| T-1 tarihli/sürümlü tarife | TestSelectTariffByStartDate, TestQuoteUsesTariffValidOnTermStart, TestV0StartMigrationPreservesReferencesAndRejectsEarlierBusiness | PASS |
| T-1 determinizm/TL dökümü/matematik | TestDeterminism, TestBreakdownSumEqualsTotal, TestMulBPHalfUp | PASS |
| T-2 araç değişikliği/iptal | TestVehicleChangeEndorsementAndCoverageOnDate, TestProRataCancellationOnStartDay | PASS |
| T-2 pro-rata/artık yıl/kısa dönem | TestCancellationOnLeapDay, TestVehicleChangeOnLeapDay, TestGoldenShortPeriodCancellation | PASS |
| T-2 geçmiş sürüm ve sınırlar | TestCancellationBoundaries, TestVehicleChangeDateBounds, TestEndorsementLinkRejectsUnrelatedEndorsement | PASS |
| T-3 hasar tarihindeki sürüm | TestClaimVersionSelectionAndCancellation | PASS |
| T-3 muafiyet→olay→teminat→yıllık | TestGoldenEventLimitsInOrder, TestEventLimitSnapshotAndPerClaimApproval, TestZeroSettlementDoesNotConsumeLimit; ADR-10 | PASS; verify T-3 olay-limiti |
| T-3 iki eşzamanlı onay/limit | TestConcurrentClaimApprovals; A6 on tekrar-race ve mutasyon, A8 tam entegrasyon | PASS |
| T-3 açıklama/adımlar | settlement.calc_steps7adım, nil olay skipped, golden/gerçek CAM onayı | PASS |
| T-4 plan/faiz/en eski taksit/iade | TestGoldenAllocation/Interest/Split, BillingInterestAllocation... | PASS |
| T-4 ≥30gün iptal | TestThirtyDayBoundary, TestBillingRunChunksMatchSingle; tekrar audit0 regresyonu | PASS |
| T-4 aynı anahtarla tek tahsilat | TestBillingPaymentIdempotency, TestBusinessRequestKeyRecovery, TestBusinessKeyRejectsChangedMeaningWithoutMiddlewareRecord | PASS |
| T-4 çift taraflı defter/mutabakat | assertBillingInvariant, ledger testleri, yeni100k batch otomatik kontrolü | PASS;100k mali mutabakat true |
| T-5 erken/plaka/atölye kuralları | TestGoldenSignalsAndThresholds, TestFraudRulesPersistenceAndAutomaticWorkflow, TestFraudPlateWindowInclusive365 | PASS |
| T-5 inceleme kuyruğu/mahremiyet | TestCustomerAndAgentClaimBodiesHideReviewInternals, fraud geçmiş testleri | PASS |
| T-5 durum makinesi/SLA | TestAdjustmentRequiresAppraisalOnEveryEntry, TestTaskLifecycleSLAAndRBAC, TestTaskCompletionTakesOwnershipAndRejectionIsWork | PASS |
| T-5 en az iş yüküne atama | TestConcurrentTaskAssignment; A6 7/7/6/race/mutasyon; A8 gerçek atama sahipliği testi | PASS |
| §5 bcrypt, access/refresh | TestPasswordsAreBcryptHashed, TestRefreshRotationAndReuseDetection | PASS |
| Tüm uçlarda erişim/kayıt kapsamı | router Authenticate/RequireRoles, access.Resolve; RBAC/404 testleri | PASS |
| Versiyonlu şema/FK/UNIQUE/CHECK/indeks | 00001–00012, TestMigrationsUpDownUp, TestRefundIndexRecoversInvalidConcurrentBuild | PASS |
| Para integer, tarih güvenliği | int64 kuruş, pricing/billing/settlement MaxInt64 testleri, caldate/leap sınırları | PASS |
| Transaction/kilit/yarış kanıtı | TestConcurrent* bağımsız HTTP/transaction; tam normal entegrasyon ve A6-race kayıtları | PASS |
| Kritik POST idempotansı | ADR-14 matrisi; ekspertiz yeni zorunlu header; geçiş expected_row_version | PASS (ADR-14 çökme sınırı) |
| Append-only audit | 00001 koruma tetikleyicisi; eski/yeni/aktör servis auditleri, görev devralma regresyonu | PASS (tetikleyiciye özel test yok) |
| Tutarlı hata/validasyon/keyset | TestErrorFormat, TestDecodeRejectsUnknownFieldsAndTrailingData, platform/page testleri | PASS |
| API kayıtlarında gerçek saat | TestAPICreatedTimestampsUseRealNow; seed/sim dışında clock.WithTime çağrısı yok | PASS |
| Context timeout/graceful shutdown | router timeout15sn, main srv.Shutdown10sn, config süre bütçeleri | UYGULANDI (kapanışa özel otomatik test yok) |
| JSON log/env/secret güvenliği | slog JSON, config env, token/parola loglanmaz, .env ignore | UYGULANDI |
| OpenAPI ve Prometheus | openapi.yaml, requests.http, TestMetricsExposed | PASS |
| §6 cp.envexample+compose/otomatik seed | Dockerfile/compose/main, TestSeedIsIdempotent, A8 mevcut100k üzerinde migration koruma kanıtı | PASS |
| Web3000/API8080/DB hosta kapalı | compose portları, health/service_healthy | PASS (önceki kurulum) |
| Soğuk önbellekte ≤5dk kurulum | Boş özel builder, farklı proje/volume77,505sn; PGtabanimajcacheistisnası | PASS (ölçüm sınırı README) |
| Root olmayan/multi-stage | backend distroless nonroot, web nginx-unprivileged | UYGULANDI |
| Deterministik ayrı simülatör100k/5y | Ayrı testler/CI PASS; v4/100k30dk26sn, parmakizi/izolasyon/mutabakat | PASS |
| Görünür canlılık | TestActivityAllRolesAndRealEvent, TestActivitySummaryOnlyFinancialRoles; UI grafik | PASS |
| §7 ≥40otomatik test | Kaynaklarda173ayrı Test* fonksiyonu; normal ve simulation etiketleri ayrı koşularda | PASS (≥40) |
| Domain≥%70/CI görünür kapsam | make coverage8paket92,1–100; check-coverage.sh / Actions summary+artifact | PASS; CI40287235419 |
| Actions build/test/lint/Docker | .github/workflows/ci.yml5iş;40287235419 | PASS |
| verify F/T/health/auth/RBAC,<15dk,nonzero | Gerçek HTTP/DB kontrolü, hata exit1, live ön kontrol exit2; yeni100k ölçümü bekliyor | PASS62/0,98,65sn |
| reset-demo tekrar denenebilir | TestSimulatorResetRemovesDatasetAndRestoresTariffs; compose stop eklendi; gerçek A8 reset PASS | PASS |
| TESTING rol/F/T/manual yollar | Bu dosya ve önceki aşama senaryoları | TAMAM |
| Bütün uçların HTTP örnekleri | requests.http/OpenAPI; appraisalheader eklendi | TAMAM |
| k6 p50/p95/p99/hata | make load20VU3dk24088istek,%0hata; READMEtablo | PASS |
| Kaos bağımlılık kesinti/iyileşme | make chaos a/b/c,0,475sn iyileşme, cleanup | PASS |
| İki indeks EXPLAIN | refund178,767→0,171ms;report382,542→34,020ms; rollback | PASS |
| En az5ADR | DECISIONS ADR-1…23, A8 ADR-10/14/17/22 güncel | TAMAM |
| §8 aday/kurulum/mimari/demo hesapları | README | TAMAM |
| README S1 en zor görev | Enes kendi metnini yazacak | KULLANICI BEKLİYOR |
| README S2 gerçek sınır | Mutlak atölye eşiği, ağır rapor süresi ve idempotans çökme aralığı | TAMAM |
| README S3 iki indeks ölçümü | docs/explain ve READMEgerçekmedyanlar | PASS |
| README S4 reddedilen AI çıktısı | Enes kendi somut örneğini yazacak | KULLANICI BEKLİYOR |
| README S5 prim altın komut/çıktı | docs/testing/pricing-golden.log ve sonverify T-1 | PASS |
| §9 anlamlı commit geçmişi | Küçük testli aşama commitleri, push kullanıcıya ait | TAMAM |
| Public repo/kullanıcı hesabı/anket23.59 | Public doğrulandı; sonpush/anket Enes'in | KULLANICIYA AİT |
| §10bonus | Ekspertiz notu ve B1 acente panosu var; TestAgentPerformanceAccessAndGoldenRows, ADR-18 tutarlılık testi, verify B1; PDF/fotoğraf yok | KISMEN |

## A8 ölçüm sırası ve güvenli çalışma

Son uygulama kodu4155bae ile100k koşusu başladı. Koşu sırasında test, build, verify, k6 ve DB/compose işlemleri yapılmaz; yalnız dosya/belge düzenlenir. Koşu başarıyla bittikten sonra sıra: summary/SQL kabulü → verify → k6 → kaos → EXPLAIN → CI sonucu → temiz kurulum →100k son verify. Yük, gün sonu ve indeks kaldırma aynı anda çalıştırılmaz.

Yük eşikleri ölçüm öncesi scripts/k6/load.js içinde: normal uçp95<1500/p99<3000ms, raporp95<5000/p99<10000ms,healthp95<500ms, hata<%1, check>%99 ve aynıanahtar aynıödeme%100. Resmî grafana/k6:2.2.0 digest9bd01d6941fca969cb61bb57d2da5ee9b385fe2aa8881df3798c196564d6ace6. `make load` setup'ta20ayrı poliçe açar, poliçe listesinde underwriter ile100k kapsamı, ölçümde1kuruş ödeme ve aynıanahtar tekrarını dener; demo verisine bu kabul kayıtları eklenir.100k iş parametresi/özetinin anlık fotoğrafı bundan önce alınır.

Kaos `make chaos`: önce ayrı poliçe; DBstop→health503/JSON5xx→aynı APIid/restartcount; DBstart→<=60sn iyileşme→başarısız anahtarla tekrar→tek ödeme/yevmiye→1200mutabakat. Finally bloğu hata dahil DB'yi up--wait ile geri açar. Ana yığın `down -v` ile kaldırılmaz.

EXPLAIN `make explain`: refund FK, rapor tarih ve workshop loss adayları; uygulama SQL'i aynı parametrelerle. Her varyantta metin ve3JSONplan docs/explain altında. İndekssiz transactionrollback,5sn kilit/120sn statement sınırı; geçersiz indeks kontrolü fail ile biter. Sorgu/planların yanında gerçek parametreler saklanır. Plan zorlayıcı enable_seqscan değişikliği yok.

Temiz kurulum: önce≥8GiB disk; ana yığın volume korunarak down; yalnız projenin api/web imaj etiketleri kaldırılır, ortak imaj/cache'e dokunulmaz. Aynı pushlanmış HEAD yeni klasöre clone, farklı compose proje adı ve yalnız ölçüme ait boş builder. `cp .env.example .env` başlangıcından up--build--wait sonuna süre; taze verify. Yalnız clone projesi down-v; main100k tekrar açılır ve son verify. Ölçüm tamamlanmadan≤5dk/PASS yazılmaz.

A8 UI tekrarında: müşteri/acente hasar listesinde UNDER_REVIEW filtre ve iç notların gizlenmesi; uzmanda red ipucu; ekspertiz eksikken ADJUSTMENT butonu pasif; ekspertiz kaydetmede Idempotency-Key; CAM olay limiti/adımı; muhasebe/UW Sistem özeti, diğer roller yalnız canlılık. A7'nin diğer375px kabulleri tarihsel kanıt olarak korunur; A8 tekrarının sonucu ayrıca kaydedilir.

### A8 son ölçüm tablosu

| Komut | Gerçek sonuç |
|---|---|
| `make test-integration` (havuz düzeltmesi sonrası) | PASS/exit0, app79,391sn |
| `make test lint build` | PASS/exit0; son uygulama kodu |
| `make coverage` (son) | PASS; sekiz paket92,1–100%; CI tablosu docs/testing/ci-domain-coverage.md |
| `make test-simulation` | GitHub40287235419 simulation işi PASS; son uygulama kodu ec8fef8 |
| `SIM_ANCHOR_DATE=2026-10-25 SIM_SEED=42 SIM_POLICIES=100000 SIM_YEARS=5 SIM_WORKERS=16 SIM_OPEN_WINDOW=48h SIM_LIVE=false make simulate` | Tekv4/100k PASS; batch1826,410sn, CLI1829,19sn |
| `/usr/bin/time -p make verify` ilk100k | PASS62/0,114,26sn |
| `/usr/bin/time -p make load` sonimaj | PASS/exit0;24088istek,20VU3dk, bütün eşikler, bütün uçlarda%0hata;CLI182,54sn |
| `/usr/bin/time -p make chaos` | a/b/cPASS; DBiyileşme0,475sn; APIrestart0;teködeme/yevmiye;toplam12,01sn |
| `/usr/bin/time -p make explain` | PASS17,09sn;3indeks rollback sonrasıvalid/ready; READMEikiindeksmedyanı |
| `cp .env.example .env` → ayrı projede `up --build -d --wait` | PASS77,505sn; boşözelbuilder/PGtabanimajcacheistisnası |
| Tazeklon `/usr/bin/time -p make verify` | PASS62/0,6,22sn |
| Ana100kson `/usr/bin/time -p make verify` | PASS62/0,98,65sn; veri/volume korundu |
| `cd backend && go test ./internal/pricing -run 'TestGolden\|TestBreakdownSumEqualsTotal\|TestDeterminism' -count=1 -v` | PASS0,496sn; docs/testing/pricing-golden.log |
| `bash -n scripts/*.sh`, YAMLparse, `git diff --check` | PASS |

İlk CI40281907975 entegrasyonFAIL, diğer4işPASS; gerçek havuz hatası düzeltildi. YeniCI40287235419 **5/5PASS**, ec8fef8. Son veri üstündeki ilk finalverify UI'da geç kapatılan seed görevini varsayılanopen filtresinde aradığı için61/1FAIL verdi (docs/testing/a8-verify-after-ui-failed.log). Betikstatus=all yapıldı; DONEgeçkapanma ihlali korunur, veri geri alınmaz. Son62/0çıktı a8-verify-final.log. K6ilkPASS before-pool-fix dosyalarında korunur; sonJSON ayrı.

### A8 gerçek tarayıcı kabulü (375×812)

Müşteri/acente İncelemede filtresinin DOMdeğeri UNDER_REVIEW; GP-H-00000003 geçmişinde içnot yok ve görünür gövdede fraud sözcüğü yok; yatay taşma yok. İçhasar uzmanı kendi sinyal/geçmişini görüyor. GP-H-00007789 ekspertizsizken ADJUSTMENTpasif, redin müşteriye gösterileceği ipucu mevcut. FRAUD_REVIEW'a hiç girmemiş GP-H-00000002'ye eksper2 arayüzden100000kuruş ekspertiz#6731 ekledi; anahtarzorunlu uçta başarı. GörevDONE/aktör7/sahip7, audit eskiatanan4→7; sonra hasaruzmanında ADJUSTMENTaktif. Yeni FRAUD_REVIEW dönüşü yapılmadı.

Muhasebe/UW100000/100000 ve46/9maliözetini görüyor; müşteriSistem yalnızcanlılık. UWödenmişCAMdosyasında19500TL→10000TL olay sınırı/9500TLkırpılma adımını gördü. Altırol kabulü (ekspertiz rolünde eksper2) yapıldı. Görseller docs/testing/ui; geçicisekme kapandı,375px viewport sıfırlandı. Normaldemo kullanıcı işlemleri DB'de bırakıldı; SIMparmakizi batchsonunu temsil eder.

### İzlenebilirlik tablosunun kapanışı

Yukarıdaki “BEKLİYOR” notları ölçüm öncesi anlık kayıttır. Son durum: yeni100k, tazekurulum, CI/kapsam, verify, k6,kaos,ikiindeks veS5primkanıtı **PASS**; READMEkurulum/mimari/ölçümler tamam.173Testfonksiyonu. Kullanıcı isteğiyle READMEkişisel1/4soruları yer tutucu, sonpush/anket kullanıcıya ait. Repo public olarak saltokunur doğrulandı. Bonuspanosu/PDF/fotoğraf ve kesilebilir genelokunabilirlik **YAPILMADI**. Önceki başarısız koşular silinmedi.


## B1 — Acente performans panosu

`bonus/acente-panosu` dalı; mevcut ADR-18 sorgusu değiştirilmedi. Acente kendi satırını, underwriter/muhasebe bütün acenteler + Doğrudan + toplamı görür. Faaliyetsiz acenteler sıfırlarla, prim <=0 oranı “—” ile görünür. Müşteri/eksper/hasar uzmanı 403, kimliksiz 401. Parametreyle başka acente veya toplam istenmesi kapsamı genişletmez.

Varsayılan TRT bugün dahil son 12 takvim ayıdır: 26 Ekim 2026 için [2025-10-27,2026-10-27). İki tarih boşaltılıp **Panoyu getir** seçilirse tüm zamanlar; tek sınır açıktır. Sütun başlıklarıyla artan/azalan sıralama yapılır; boş oranlar sonda kalır. Yükleniyor, hata/yeniden deneme ve boş faaliyet durumları mevcuttur. Frontend'de mevcut otomatik test deseni/çalıştırıcısı yok; yeni paket kurulmadı.

### Performans ve 100k tutarlılık

[Ham EXPLAIN ve ölçümler](docs/explain/b1-after.json): yeni indeks/migration yok. Tüm zamanlar 2113,342 / 501,841 / 443,704 ms; medyan **501,841 ms** (<1 sn PASS). Varsayılan 12 ay 250,441 / 257,549 / 278,072 ms; medyan **257,549 ms**. İlk örnek saklandı; soğuk önbellek veya yük altında alt sınır garantisi verilmez. Önceki 3972,337 ms tek ölçüm medyan değildir.

Aynı REPEATABLE READ READ ONLY snapshot'ında pano toplamları ve hareketli satırlar ADR-18 ile birebir eşittir; sıfır faaliyetli satırlar ek olarak korunur. Kayıt [b1-consistency.json](docs/testing/b1-consistency.json), ham yanıtlar b1-consistency-*-raw.json.

| Dönem | Poliçe | Yazılan net prim (kuruş) | Tahakkuk eden tazminat (kuruş) | Eşitlik |
|---|---:|---:|---:|---|
| Tüm zamanlar | 100085 | 71053192211 | 18240008148 | PASS |
| Son 12 ay | 19530 | 15834741154 | 4973901030 | PASS |
| [2026-09-26,2026-10-26) | 1062 | 1040036279 | 571862630 | PASS |

Bu tabloda 100000 SIM ile mevcut demo/kabul kayıtları birlikte raporlanır; 100085 toplam poliçe sayısı, SIM hedefinin değiştiği anlamına gelmez. Bu ölçümler verify'ın sonraki demo mutasyonlarından öncedir.


### B1 son doğrulama ve tarayıcı kabulü

İstenen sıra son kodda tekrar tamamlandı (önceki başarılı koşular da b1-validation.json ve b1-*.log içinde korunur):

| Komut | Gerçek sonuç | Süre |
|---|---|---:|
| `make test lint build` | PASS; testli 20 Go paketi, vet/ESLint/TypeScript/Vite | 7.685 sn |
| `make test-integration` | PASS; testli 21 paket; app 88,777 sn | 92.566 sn |
| `docker compose up --build -d --no-deps --wait api web` | PASS; yalnız api/web, DB konteyneri korundu | 13.084 sn |
| `make verify` | PASS; 64 PASS / 0 FAIL, B1 rol ve tutarlılık dahil | 108.895 sn |

İlk tarayıcı denemesinde alanlar boş görünürken gönderimin eski tarihleri kullandığı görüldü (FAIL, b1-ui-first-failure.json). FormData ile güncel alan değerleri tek kaynaktan gönderilerek düzeltildi; son sıralı testler ve yeniden derlenen tarayıcı kabulü PASS. Hiçbir başarısız kayıt silinmedi.

375 px gerçek tarayıcı: document.scrollWidth = innerWidth = 375; tablo kabı 341 px, iç tablo 780 px. AGENT bir satır ve total yok; UNDERWRITER 21 acente ve bir toplam satırı görüyor. Ana veri kümesinde acentesiz poliçe olmadığı için Doğrudan satırı yok; bu satır geçici DB entegrasyon fixture'ında doğrulanır. Masaüstü 1440/1440; 21 satır + toplam. Artan/azalan poliçe adedi sıralaması, boş tarihle tüm zamanlar, boş dönem/sıfır primde “—”, hatalı tarihte error/yeniden deneme ve yükleniyor durumu gerçek tarayıcıda görüldü. Viewport sıfırlandı, kabul sekmesi kapatıldı. [Tarayıcı kaydı](docs/testing/b1-ui.json), [mobil](docs/testing/b1-ui-mobile.png), [masaüstü](docs/testing/b1-ui-desktop.png).

Gerçek HTTP üçer isteğin medyanı: tüm zamanlar **432,560 ms**, son 12 ay **229,244 ms**, dar dönem **56,785 ms**. Üç dönemin dört metrikli toplamı ve hareketli satırları ADR-18 ile birebir aynı; [HTTP kayıtları](docs/testing/b1-http-consistency.json). Bu, verify sonrası mevcut demo/kabul kayıtlarıyla ayrı bir ölçümdür; önceki SQL tablosunun sayılarıyla farklı anlar karıştırılmaz.

100k önce/sonra A8.1 yöntemiyle PASS: 100000 SIM poliçe, 7780 hasar, 6727 ekspertiz, 495 işaretli, 46 açık inceleme, 46 açık görev aynı. Gerçek saate bağlı SLA ihlali 46→46; düşüş yok. Kayıtlı parmak izi, eski ekspertiz alanlarının MD5'i ve volume kimliği aynı. Parmak izi yeniden hesaplanmadı. measured_at ve database_bytes farkları yalnız raporlandı. [Karşılaştırma](docs/testing/b1-comparison.json); ham b1-acceptance-before/after.json ve b1-preservation-before/after.json. Verify olağan demo/kabul işlemleri ekler; SIM iş alanları değişmedi. Simülasyon veya yeni 100k koşusu yok.
