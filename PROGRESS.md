# PROGRESS

## Durum

| Aşama | Durum | Not |
|---|---|---|
| A0 | TAMAMLANDI | Plan onaylandı |
| A1 | TAMAMLANDI | Docker, migrasyon, seed, auth, RBAC, verify iskeleti |
| A2 | TAMAMLANDI | T-1 prim motoru, F-1 teklif/onay/poliçeye çevirme, idempotans, audit, keyset |
| A3 | TAMAMLANDI (kullanıcı doğruladı) | T-2/F-2: sürüm değişmezliği (00003), zeyil, iptal, gün esaslı/kısa dönem iade, tarihte teminat |
| A4 | TAMAMLANDI | T-3/F-4/F-5/F-6; hasar, ekspertiz, tazminat, ödeme ve paralel limit kanıtı |

## Şema notları (uygulama kuralı olarak ele alınacak)

`00001` değiştirilmedi. `00002_schema_fixes.sql` ile 1) ters kayıt tekilliği, 2) `claims (policy_id, policy_version_id)` bileşik FK, 3) `quotes (agent_id, id)` indeksi eklendi. Kalan bulgular:

1. **Satırsız yevmiye kaydı:** denge tetikleyicisi yalnızca `journal_lines` INSERT'inde çalışır. A4 `ledger.Post` satırsız/dengesiz kayıtları kodda reddeder; geçersiz hesapta transaction geri alma testi var.
2. **`endorsements.from_version_id` / `to_version_id`:** sürümün aynı poliçeye ait olması uygulama kuralı (A3).
3. **`claims.status` ↔ `claim_status_history`:** aynı transaction'da, `row_version` kontrolüyle tek servisten (A4). İlk kayıt `NULL → REPORTED`.
4. **`claim_settlements.CANCELLED` ↔ `claim_transitions`:** `APPROVED`'dan geri dönüş yok; uzlaşma iptalinde hasar durumu tanımsız. **A4 kararı:** uzlaşma iptali yok; APPROVED geri alınamaz (ADR-11).
5. **`policy_limit_balances`:** satırlar poliçe düzenleme/zeyilde uygulama tarafından açılmalı; zeyilde limit < kullanılan olursa `CHECK` hatası 422 domain hatasına çevrilmeli; dönemin poliçe tarihleri içinde olması kodda doğrulanmalı (A2–A4).
6. **`idempotency_keys`:** PK `(user_id, idem_key)`; `request_hash` method + path + gövdeyi kapsamalı (farklıysa 422). Süre/TTL kolonu yok: takılı `IN_PROGRESS` için uygulamada zaman aşımı kuralı (A2).
7. **Seed/reset:** `accounts` ve `claim_transitions` migrasyonda doldurulur, `reset-demo` bunları korur (`scripts/reset-demo.sh` KEEP listesi). Yeni migrasyon başvuru verisi eklerse listeye eklenmeli. Simülatör için 11 haneli deterministik `national_id` üretimi gerekecek (A7).
8. **İade toplamı:** `payments` iade toplamının ödemeyi aşmasını engellemez; kilit + toplam kontrolü A5'te. Taksit `status`/`paid_kurus` tutarlılığı yalnızca `PAID` için zorlanıyor.
9. **`policies`:** `CANCELLED` için `cancel_effective_date`/`cancel_reason` zorunlu değil (kodda zorunlu tutulacak); `EXPIRED` geçişini yapan iş yok.

## README doğrulama sorusu 5 — prim motoru kanıtı (gerçek çıktı, 25 Ekim 2026)

```bash
cd backend && go test -count=1 -v -cover -run 'TestGolden|TestDeterminism|TestBreakdownSumEqualsTotal|TestMulBPHalfUp|TestSelectTariffByStartDate|TestAgeRangeBoundaries' ./internal/pricing/
```

```
--- PASS: TestGoldenKasko (0.00s)
--- PASS: TestGoldenKonut (0.00s)
--- PASS: TestDeterminism (0.00s)
--- PASS: TestBreakdownSumEqualsTotal (0.12s)
--- PASS: TestAgeRangeBoundaries (0.00s)
--- PASS: TestMulBPHalfUp (0.00s)
--- PASS: TestSelectTariffByStartDate (0.00s)
PASS
coverage: 81.4% of statements
ok  	github.com/EnesGoker/guvenpolice/backend/internal/pricing	0.725s	coverage: 81.4% of statements
```

(Bu filtreyle %81,4; paketin tüm testleriyle %95,5.) Uçtan uca: `make verify` çıktısında

```
  PASS  T-1 determinizm             aynı girdi iki kez → aynı döküm/prim/hash (1514382 kuruş)
  PASS  T-1 dokum-toplami           satır toplamı 1514382 = prim 1514382
  PASS  T-1 tarife-tarihi           2026-06-30→v1 (1188663), 2026-07-01→v2 (1514382)
```

## Bilinen sınırlamalar

- **Geriye dönük zincir yok (A3):** zeyil tarihi açık sürümün başlangıcından sonra olmalı; son zeyilden önceki bir tarihe zeyil yapılamaz (422). Açık sürüm içinde geçmiş bir tarihe zeyil ise yapılabilir — A4'te o tarihte/sonrasında REJECTED olmayan hasar varsa zeyil/iptal reddedilir (CLAIMS_AFTER_EFFECTIVE_DATE).
- **Başlangıç gününde iptal (A3):** sıfır uzunlukta sürüm aralığı CHECK ile yasak olduğundan açık sürüm kapatılamaz; iptal edilmiş poliçede `effective_to` NULL bir sürüm kalır ve teminat poliçe düzeyindeki `cancel_effective_date` ile sona erer (`calc_detail.version_closed = false`). Tarihte teminat sorgusu ve A4 hasar seçimi bu yüzden her zaman `policy.VersionOn`'u (poliçe sınırları + iptal tarihi) kullanmalı.
- **Limit değişikliği primi etkilemez (A3):** örnek tarifede prim teminat başına sabit tabandan hesaplanır; limit yalnızca katalog üst sınırına kadar değiştirilebilir. Yıllık toplam limit zeyilde değişmez.

- **İdempotent tekrar yanıtında anahtar sırası:** `Idempotency-Key` ile tekrarlanan istekte saklı yanıt `idempotency_keys.response_body` (JSONB) kolonundan döner. JSONB nesne anahtarlarını kendi sırasına göre sakladığı için tekrar yanıtındaki anahtar sırası ilk yanıttan farklı olabilir; alanlar ve değerler birebir aynıdır (yanıt `json.Compact` ile sıkıştırılır, `Idempotent-Replayed: true` başlığı eklenir). Bayt düzeyinde aynılık gerekirse kolon `TEXT/BYTEA` olmalı — şema kullanıcıya ait olduğu için değiştirilmedi (A3 kararı: kabul).
- **Kısıt ihlalleri:** asıl koruma kilit + durum kontrolüdür; `policies.quote_id`/`policy_no` benzersizlik ihlali ikinci savunma hattı olarak 409 `QUOTE_ALREADY_CONVERTED`'e çevrilir (A3-0). Mutasyon denemesi (convert'teki `FOR UPDATE` kaldırıldı, eşzamanlı 5 istek, 5 tekrar): kaybedenler 500 yerine 409 aldı; kilit geri konuldu.

## A3 notları

- **A3 tarihsel notu (A4'te çözüldü) — `policy_versions.endorsement_id`:** `policy_versions.endorsement_id ↔ endorsements.to_version_id` döngüsel FK'leri ertelenebilir değil ve araç değişikliğinde `to_version_id` zorunlu (CHECK). Alanı doldurmanın tek yolu sürümü ekledikten sonra UPDATE etmekti; A3 kararı (yalnızca `effective_to` kapatılabilir) buna izin vermiyor. Alan NULL kalıyor, bağ `endorsements.from_version_id/to_version_id` ile kuruluyor. Doldurulması istenirse 00003 tetikleyicisi `endorsement_id NULL → değer` geçişine bir kez izin verecek şekilde genişletilebilir (kullanıcı kararı).
- Şema bulgusu 2 (`endorsements.from/to_version_id` sürümün aynı poliçeye ait olması) uygulamada sağlanıyor: zeyil/iptal yalnızca kilitli poliçenin kendi açık sürümünden üretiliyor.
- Şema bulgusu 5 (zeyilde limitin kullanılanın altına düşmesi) ele alındı: 422 `LIMIT_BELOW_USAGE`, bakiyede koşullu UPDATE.
- Şema bulgusu 9 (`CANCELLED` için iptal tarihi/gerekçe zorunluluğu) kodda sağlanıyor: iptal her zaman `cancelled_at`, `cancel_effective_date`, `cancel_reason`'ı birlikte yazar.
- Zeyiller (`endorsements`) veritabanında append-only değil (görevde istenmedi); uygulama yalnızca INSERT yapıyor. İstenirse 00001'deki `forbid_modify()` ile tek satırlık bir tetikleyici eklenebilir.
- `POLICY_NOT_ACTIVE` kodu `EXPIRED` için ayrıldı; `EXPIRED` durumunu atayan iş hâlâ yok (kapsam dışı).

## A2 notları

- Şema bulgusu 5 (`policy_limit_balances`) kısmen ele alındı: bakiyeler poliçe düzenlenirken her teminat + yıllık toplam için poliçe dönemiyle açılıyor. Zeyilde limit düşürme kuralı A3/A4'te.
- Şema bulgusu 6 (`idempotency_keys`) ele alındı: `request_hash` = method + path + gövde; süre aşımı `IDEMPOTENCY_STALE_AFTER`. Eski kayıtların temizliği (TTL silme işi) **yok**; `idempotency_keys_created_idx` hazır, A8'de küçük bir temizlik eklenebilir.
- `EXPIRED` durumu otomatik atanmıyor; süresi dolmuş teklif çevrilmek istenirse 422 `QUOTE_EXPIRED` dönüyor.
- `GET /api/v1/policies/{id}` (tek poliçe detayı) yok; A3'te sürüm geçmişiyle birlikte eklenecek.
- Arayüzde teklif formu müşteri listesini tek sayfada (en fazla 100) yükler; daha fazla müşteri için arama yok.
- Eşzamanlı convert'te kilit yoksa `policies.quote_id` UNIQUE ikinci güvence olur; A2'de 500 dönüyordu, A3-0 ile 409 `QUOTE_ALREADY_CONVERTED` (kalıcı test: `TestConvertUniqueViolationIsConflict`).

## Teknik notlar

- **Go:** yerelde `go1.27.3 darwin/arm64` (`/usr/local/go/bin`; ajan kabuğunun PATH'inde yoktu, `make GO=/usr/local/go/bin/go` ile de çağrılabilir). `go.mod` `go 1.27.3`, Docker `golang:1.27.3-alpine`; ikisi de çekilip derlendi.
- **npm 10.8.2 hatası:** `npm install` arborist içinde `Cannot read properties of null (reading 'edgesOut')` ile düştü (peer çözümleme). `frontend/.npmrc` içinde `legacy-peer-deps=true` ile kuruldu; `npm ci` Docker'da da aynı ayarla çalışıyor. TypeScript, `typescript-eslint` peer aralığı (<6.1) nedeniyle 5.9.3'e sabitlendi.
- **Docker imajları:** api 23,2 MB (distroless static, `nonroot`), web 76,3 MB (nginx-unprivileged, uid 101).


## A4 (ajan: Codex)

**Durum: TAMAMLANDI.** A1–A3 kalıpları ve çalışan testler korundu; 00001/00002/00003 değiştirilmedi. İlk commit yalnızca onaylı CURRENT_TASK.md oldu (`3a610bb`, `docs: A4 görev tanımı`). A5'e başlanmadı, push yapılmadı.

### Yapılanlar

- 00004: `endorsement_id` bir kez NULL → değer; zeyil transaction'ında bağlantı yazılır. Up/Down/Up ve ikinci yazmayı reddetme testi.
- Hasar bildiriminde poliçe kilidi + mevcut `policy.VersionOn`; müşteri/acente kapsamı, gelecek tarih/teminat/atölye kontrolü, sunucuda sürüm/plaka seçimi. Bildirim/geçmiş/audit tek transaction.
- Tek `claim.Transition`: claim FOR UPDATE, expected row_version, izinli geçiş ve red gerekçesi; durum/geçmiş/audit birlikte. Eksper kapsamı APPRAISAL veya kendi ekspertizi; son eklenen ekspertiz hesapta kullanılır.
- Saf settlement: altı açıklamalı adım, sabit/yüzde muafiyet, teminat/yıllık bakiye kırpması ve güvenli tam sayı yuvarlama. Claim → bakiyeler id ASC kilidi + koşullu UPDATE; settlement/tüketim/geçiş/audit/yevmiye atomik.
- Onay ve ödeme idempotent; `ledger.Post` satır/denge kontrolü yapar. Muhasebe 5000→2000 onay, 2000→1000 ödeme; defter dengesi ve geri alma testi.
- Altı rol için hasar ekranı: müşteri/acente bildirim, hasar uzmanı geçiş/önizleme/onay, eksper formu, muhasebe ödeme, underwriter salt okunur liste/detay. İşlem sonrası liste yenilenir.
- OpenAPI 0.4.0, requests.http, ADR-10/11, TESTING ve ROADMAP kanıtları güncellendi.

### Testler ve gerçek sonuçlar

- `make test`: PASS (52 birim test fonksiyonu).
- `./scripts/test-integration.sh -coverpkg=./internal/claim,./internal/ledger,./internal/settlement -coverprofile=/tmp/guvenpolice-a4-coverage.out`: PASS (40 entegrasyon test fonksiyonu; gerçek geçici PostgreSQL 16; 00004 dahil Up/Down/Up).
- `(cd backend && go tool cover -func=/tmp/guvenpolice-a4-coverage.out)`: bu üç paket toplam %81,5; claim %79,9, ledger %90,9, settlement %100.
- `./scripts/test-integration.sh -run TestConcurrentClaimApprovals -count=10`: PASS.
- `./scripts/test-integration.sh -run TestConcurrentClaimApprovals -count=10 -race`: PASS. İki dosya × yıllık limitin %80'i; 800.000 + 200.000 = 1.000.000.
- Mutasyon komutu `./scripts/test-integration.sh -run TestConcurrentClaimApprovals -count=1 -v`: bakiye FOR UPDATE geçici kaldırılmışken beklenen FAIL; ikinci onay `409 LIMIT_CHANGED`, kullanım 800.000; koşullu UPDATE yakaladı. Kilit geri konuldu; 10 tekrar `-race` PASS.
- `make lint`, `make build`: PASS. `make up`: db/api/web healthy. `make verify`: 40 PASS, 0 FAIL (eski A1–A3 kontrolleri dahil). Gerçek katalog yarışında 147.000.000 + 33.000.000 = 180.000.000 yıllık limit.
- Ruby YAML ayrıştırması ve 202 iç OpenAPI referansı: PASS. Yeni dış OpenAPI linter: ÇALIŞTIRILMADI (araç kurulmadı).
- Tarayıcı: müşteri → hasar uzmanı → eksper → hasar uzmanı → muhasebe; GP-H-00000007 PAID, 150.000 kuruş, ödeme yevmiyesi #14. 375 px viewport'ta sayfa/içerik genişliği 375; yatay taşma yok. Boş liste ve işlem sonrası liste yenilenmesi doğrulandı. Görüntü: `/Users/macair/.codex/visualizations/2026/10/25/01a1736b-a440-73e3-9306-e324601dce63/a4-paid-375.png` (yerel kanıt, Git'e eklenmedi). Acente kendi listesini, underwriter salt okunur tüm dosyaları gördü; teminat olmayan tarihte hata ve kapalı bildirim butonu doğrulandı.

### Bilinen eksikler / riskler

- Uzlaşma iptali ve ayrı olay limiti yok (onaylı A4 kararı/iş varsayımı); APPROVED geri alınamaz. Fraud/SLA/atama A6; taksit/tahsilat/prim muhasebesi A5.
- İdempotans JSONB anahtar sırası ve iş transaction'ı–yanıt saklama aralığı sınırlaması sürüyor. Yeni anahtarla yeniden mali işlem durum/benzersizlik denetiminde reddedilir.
- A3'te oluşturulmuş sürümlerin NULL endorsement_id alanlarına geriye dönük veri yaması yapılmadı; yeni zeyiller doldurur, eski bağlantı endorsements üzerinden okunur.
- Test verileri kurgudur; gerçek sigorta tarifesi/mevzuatı değildir. A4 kapsamı genişletilmedi.

### Değişen dosyalar (tam yollar)

| Dosya | Amaç |
|---|---|
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/CURRENT_TASK.md` | Kullanıcının onaylı A4 görev tanımı; ilk commit. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/DECISIONS.md` | ADR-8 güncellemesi ve ADR-10/11 limit, kilit, mali kayıt kararları. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/PROGRESS.md` | A4 devir/teslim durumu, kanıtlar, tam dosya listesi ve sınırlamalar. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/ROADMAP.md` | A4 kutuları ve F-4/F-5/F-6/T-3 kanıt matrisi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/TESTING.md` | A4 komutları, gerçek sonuçlar, rol senaryoları ve mutasyon kanıtı. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_integration_test.go` | Yedi gerçek PostgreSQL testi: akış, yarış, sürüm, koruma, RBAC, sıfır tutar, rollback. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/integration_test.go` | 00004 dahil migration sürümü ve Up/Down/Up. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/policy_integration_test.go` | Zeyil bağlantısının bir kez doldurulduğunu doğrulama. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/router.go` | Yeni hasar uçları, rol ve idempotans middleware bağlantıları. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/handler.go` | Hasar HTTP giriş/yanıt ve liste doğrulaması. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/repo.go` | Dosya, geçmiş, ekspertiz ve settlement veri erişimi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/service.go` | Bildirim, kapsam, dosya takibi ve ekspertiz uygulama kuralları. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/settlement.go` | Kilitli onay, koşullu limit tüketimi, ödeme ve yevmiye transactionları. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/transition.go` | Tek durum makinesi; row_version, geçmiş ve audit. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/ledger/ledger.go` | Satırlı/dengeli yevmiyeyi çağıranın transactionında yazma. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/ledger/ledger_test.go` | Geçersiz/satırsız/dengesiz ve taşan kayıtları reddetme. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/change.go` | Endorsement bağlantısını doldurma ve hasardan önceye zeyil/iptal koruması. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/settlement/settlement.go` | Saf, tam sayı ve açıklanabilir tazminat hesabı. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/settlement/settlement_test.go` | Elle hesaplı altın örnekler, sınır ve doğrulama testleri. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00004_policy_endorsement_link.sql` | Bağlantıya tek NULL→değer istisnası; tersine çevrilebilir yeni migrasyon. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/api/types.ts` | Hasar, ekspertiz, geçmiş ve hesap adımı tipleri. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/ClaimDetail.tsx` | Geçmiş, ekspertiz, rol geçişleri, hesap, onay ve ödeme detayı. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/ClaimForms.tsx` | Tarih/sürüm teminatlı bildirim ve ekspertiz formları. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/PolicyDetail.tsx` | Poliçe detayından hasar bildirimine giriş. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/format.ts` | Hasar durumlarının Türkçe etiketleri. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/ClaimsPage.tsx` | Durum filtresi, keyset liste ve dosya seçimi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/Dashboard.tsx` | Hasar/ekspertiz ekranlarının rol menüsüne bağlanması. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/roles.ts` | Altı rol için hazır hasar menüsü. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/openapi.yaml` | A4 istek/yanıt şemaları, yeni uçlar ve hata sözleşmesi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/requests.http` | Hasar→ekspertiz→onay→ödeme→kapatma örnek HTTP istekleri. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/verify.sh` | F-4/F-5/F-6/T-3 gerçek API ve arka planda paralel curl kanıtı. |

### Yerel commitler

- `3a610bb docs: A4 görev tanımı`
- `0b405a3 fix(policy): zeyil bağlantısını bir kez doldur ve hasarlı tarihlere değişikliği engelle`
- `44da754 feat(settlement): adımlı tam sayı tazminat hesabı ve dengeli yevmiye yardımcısı`
- `7890706 feat(claim): hasar, ekspertiz, limit kilitli onay ve idempotent ödeme`
- `9eb4a9a feat(web): altı rol için hasar dosyası, ekspertiz ve tazminat akışı`
- `f831bef test(claim): yarış testini kilitte buluştur ve giriş/geri alma kontrollerini güçlendir`
- `08be802 fix(web): hasar işlemlerinden sonra dosya listesini yenile`
- `100a267 feat(verify): F-4 F-5 F-6 ve paralel yıllık limit kanıtları`
- `a13b820 style(claim): yeni paketlerde mevcut import gruplarını uygula`

Son doküman/biçim commitleri `git log 4643ee6..HEAD` ve teslim raporunda yer alır; push yok.

### Sonraki görev

Yalnızca öneri: A5 — T-4/F-3 taksit, tahsilat ve prim muhasebesi. Yeni kullanıcı onayı olmadan başlanmaz.


## A5 (ajan: Codex) — 25 Ekim 2026

### Durum

**TAMAMLANDI.** Onaylı T-4/F-3 kapsamı uygulandı ve doğrulandı. A6'ya geçilmedi. A4 claim/settlement koduna veya 00001–00004 migrationlarına dokunulmadı; mevcut yardımcılar ve paket kalıpları korundu. Yeni bağımlılık kurulmadı, veri sıfırlanmadı, push yok.

### Yapılanlar

- DB'siz billing hesabı, ay sonu vadeleri, half-up basit faiz, faiz→anapara tahsisi, zeyil/iptal için tek prim ayarı.
- Issue/convert ile atomik plan/taksit/yevmiye; mevcut zeyil/iptal transactionına mali ayar ve ayrı cash_refund_kurus. payment/store ayrımı import döngüsü oluşturmuyor.
- Önce poliçe satırı kilidi: ödeme/iade/tahakkuk/plan; gün sonu SKIP LOCKED ve mevcut CancelTx ile NON_PAYMENT. Mizan repeatable-read snapshot ve ortak mutabakat değişmezi.
- İdempotent gecikmiş A/B seed, minimal rol arayüzleri, gerçek API verify ve belgeler.
- 00005 yalnızca plan toplamı CHECK >0 → >=0; Down riski yorumda. Kullanıcının yerel CURRENT_TASK değişikliği ilk docs commitinde korundu.

### Testler ve gerçek sonuçlar

| Tam komut / kontrol | Sonuç |
|---|---|
| `make test` | PASS; 58 birim test fonksiyonu. |
| `(cd backend && go test -count=1 -cover ./internal/billing ./internal/caldate)` | PASS; billing %92,1, caldate %100. |
| `./scripts/test-integration.sh -coverpkg=./internal/payment/...,./internal/billing,./internal/ledger -coverprofile=/tmp/guvenpolice-a5-coverage.out` | PASS; 52 entegrasyon fonksiyonu (11 yeni A5); A1–A4 korunuyor; 00005 dahil Up/Down/Up. |
| `(cd backend && go tool cover -func=/tmp/guvenpolice-a5-coverage.out)` | Birleşik toplam %83,3; payment %80,6, store %81,0, ledger %83,9, billing %93,1 (birim+entegrasyon). |
| `./scripts/test-integration.sh -run TestConcurrentPayments -count=10 -race -v` | PASS; bağımsız iki HTTP/transaction, gate ile kilitte buluşma, 201 + 422 OVERPAYMENT. |
| `./scripts/test-integration.sh -run TestConcurrentPayments -count=1 -v` (geçici kilit mutasyonunda) | Beklenen FAIL; 201 + 409 CONCURRENT_PAYMENT. installments CHECK paid<=amount ikinci savunması yakaladı, bakiye 10000 kaldı. |
| `./scripts/test-integration.sh -run TestConcurrentPayments -count=10 -race` (kilit geri konulduktan sonra) | PASS; kaynak finally ile geri yüklendi, mutasyon kalıcı değil. |
| `make lint` | PASS; normal/integration go vet ve ESLint. |
| `make build` | PASS; Go ve TypeScript/Vite. |
| `make up` | PASS; db/api/web healthy, 00005 ve iki demo poliçesi yüklendi. |
| `make verify` | İki ayrı çalıştırmada 47 PASS, 0 FAIL; ikinci tur iptal edilmiş seed A ile de geçti. |
| `./scripts/test-integration.sh -run TestBillingInterestAllocationRefundAndDates -count=1` | PASS; ek faiz tahsisi iade kontrolü. |
| `bash -n scripts/verify.sh`, `git diff --check` | PASS. |
| Yerel Ruby YAML + iç $ref taraması | PASS; 245 referans çözüldü. Dış OpenAPI linter ÇALIŞTIRILMADI (yeni araç kurulmadı). |

İade/açılan anapara, eşzamanlı iade, iptal sonrası tahsilat, 29/30 sınırı, seed faiz altın hesabı, kilitli kayıt atlama, A4 hasar koruması, zeyil +/−, sıfır plan ve nakit iade, RBAC ve keyset test edildi. Her A5 mali senaryosu sonunda tüm defter/yevmiye dengesi, satırsız kayıt yokluğu, plan toplamları ve 1200 = açık anapara + faiz kontrol edildi.

Tarayıcı (375 px): acente yeni teklifi üç taksitle çevirdi: 1.481.480 kuruş = 493.828 + 493.826 + 493.826; muhasebe 100 kuruş tahsilat/40 kuruş iade, müşteri kendi poliçesine 1 kuruş ödeme ve fazla ödeme hata görünümü; muhasebe aynı gün gün sonu 0 yeni faiz/iptal ve dengeli mizan. Boş durumlar göründü. Ölçüm viewport=375, scrollWidth=375; yatay taşma yok. Yerel ekran kanıtı (Git dışında): `/Users/macair/.codex/visualizations/2026/10/25/01a1736b-a440-73e3-9306-e324601dce63/a5-billing-375.jpg`.

### Bilinen eksikler / riskler

- Kurgusal muhasebe: prim anında gelir, kazanılmamış prim karşılığı yok; gerçek tarife/mevzuat veya banka entegrasyonu değildir.
- Manuel iade yalnızca ACTIVE poliçede; otomatik zamanlayıcı yok. Her gün sonu poliçesi ayrı commit; sonraki poliçe hatası önceki başarıları geri almaz.
- Plansız A5 öncesi veri için 409 BILLING_PLAN_MISSING, otomatik backfill yok; reset-demo yalnızca silinebilir demo için kullanıcı işlemidir. 00005 Down sıfır plan varsa başarısız olabilir; eski migrationlar değişmedi.
- Mevcut idempotans middleware'inde iş commit'i–yanıt saklama aralığında süreç çökmesi riski devam ediyor; normal tekrar/eşzamanlı istekler tek ödeme üretir.
- B seed tarihi ilk oluşturulduğu güne bağlı; zaman ilerleyince kendisi de >=30 gün kuralına girer. Verify kayıt bazında çoklu faizi destekler, başlangıç B örneği için taze demo gerekebilir.

### Değişen dosyalar (tam yollar)

| Dosya | Amaç |
|---|---|
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/.env.example` | Kurgusal günlük faiz env varsayılanı. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/CURRENT_TASK.md` | Kullanıcının onaylı A5 görev tanımı; ilk commit. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/DECISIONS.md` | ADR-12/13 kilit, tahsis, faiz ve muhasebe kararları. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/PROGRESS.md` | A5 teslim durumu, kanıtlar, riskler ve tam dosya listesi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/ROADMAP.md` | A5 çıkış kapısı ve F-3/T-4 kanıt matrisi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/TESTING.md` | A5 komutları, hesap örnekleri, yarış/mutasyon ve UI kanıtları. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/cmd/api/main.go` | Faiz oranını router bağımlılıklarına aktarır. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/integration_test.go` | 00005 migration sürümü ve Up/Down/Up doğrulaması. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/payment_integration_test.go` | 11 mali entegrasyon senaryosu ve ortak plan/defter/mutabakat değişmezi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/router.go` | Ödeme/iade/gün sonu/mizan/yevmiye uçları; mevcut RBAC ve idempotans. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/billing/billing.go` | Saf taksit, faiz, tahsis, tek prim ayarı ve 30 gün hesabı. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/billing/billing_test.go` | Elle hesaplanmış altın örnekler ve sınır testleri. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/caldate/caldate.go` | Ay sonuna kırpılan AddMonths. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/caldate/caldate_test.go` | 29 Şubat, ay sonu ve yıl sınırı örnekleri. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/config/config.go` | LATE_INTEREST_DAILY_PPM doğrulaması ve varsayılan 1000. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/ledger/handler.go` | Repeatable-read mizan ve keyset yevmiye HTTP girişleri. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/ledger/query.go` | Hesap/yevmiye dengesi, 1200 mutabakatı ve satırlı liste. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/handler.go` | Plan, tahsilat, iade ve gün sonu HTTP sözleşmesi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/repo.go` | Kilitli mali görünüm; taksit, faiz, ödeme, tahsis ve iadeler. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/run.go` | Poliçe bazında SKIP LOCKED tahakkuk ve ortak CancelTx iptali. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/service.go` | Poliçe kilitli tahsilat/iade; tarih, sahiplik, audit ve hata eşleme. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/accrual.go` | Tam sayı faiz kaydı ve LATE_INTEREST yevmiyesi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/adjustment.go` | Ortak prim ayarının kalıcı etkisi ve tahsissiz nakit REFUND. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/payment.go` | Ödeme, pozitif tahsis ve PAYMENT/REFUND yevmiyesi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/plan.go` | Plan yükleme/oluşturma ve POLICY_ISSUE yevmiyesi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/change.go` | Zeyil/iptal transactionına mali ayar; nakit iade önizlemesi; ortak CancelTx. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/policy.go` | Issue ile aynı transactionda plan/taksit/yevmiye; seed için dahili sabit no. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/quote/handler.go` | Convert isteğe bağlı 1–12 installment_count doğrulaması. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/quote/service.go` | Taksit sayısını mevcut Issue akışına aktarır; varsayılan 1. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/billing.go` | Issue üzerinden sabit no ile iki idempotent gecikmiş demo poliçesi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/demo.go` | Mevcut seed transactionından mali demo servisini çağırır. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00005_installment_plan_zero_total.sql` | Yalnızca total_kurus CHECK >=0; Down riski yorumlu. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/docker-compose.yml` | Günlük faiz oranını API ortamına aktarır. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/api/types.ts` | Mali plan/ödeme/faiz/mizan/yevmiye ve nakit iade tipleri. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/BillingPanel.tsx` | Plan, faiz, tahsis, tahsilat ve iade formları. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/PolicyChangeForms.tsx` | Önizlemede prim azalışından ayrı müşteriye nakit iade. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/PolicyDetail.tsx` | Yetkili rollerde mali plan bileşeni. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/QuoteDetail.tsx` | Convert için taksit sayısı ve değişen girdide yeni idempotans anahtarı. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/AccountingPage.tsx` | Gün sonu, mizan/mutabakat ve keyset yevmiye ekranı. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/Dashboard.tsx` | Muhasebe ekranı ve müşteri ödeme gezinmesi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/roles.ts` | A5 tahsilat menüsünü kullanılabilir yapar. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/openapi.yaml` | A5 uçları, istek/yanıt/hata şemaları ve convert/cash_refund sözleşmesi. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/requests.http` | Mevcut token/istek değişkenleriyle A5 örnekleri. |
| `/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/verify.sh` | F-3/T-4 API, arka planda paralel curl, kayıt bazında awk faiz ve mutabakat kanıtı. |

### Yerel commitler

- `5776e23 docs: A5 görev tanımı`
- `2ddbdeb feat(billing): saf taksit, faiz, tahsis ve prim ayarı hesapları`
- `fbe7235 feat(policy): düzenleme ve zeyil iptal akışını ödeme planı ve deftere bağla`
- `496faac feat(payment): tahsilat, iade, gün sonu ve defter mutabakatı`
- `f577e02 feat(web): taksit seçimi, ödeme iade ve mizan ekranları`
- `ff2ee01 test(verify): F-3 tahsilat ve T-4 paralel ödeme faiz mutabakat kanıtları`

- `a30c5ea test(payment): manuel iadede faiz tahsisinin yeniden açılmasını doğrula`

Son A5 doküman commitinin hash'i `git log 7fa6b14..HEAD` ve teslim raporunda bulunur; push yapılmadı.

### Sonraki görev

Yalnızca öneri: A6 — fraud, atama/SLA ve raporlar. Yeni onay olmadan başlanmaz.


## A6 (ajan: Codex)

### Ara durum — 0. bölüm kapısı

0.1–0.10 TAMAMLANDI; T-5/F-7 henüz bu kontrol noktasında uygulanmadı. Ek kullanıcı kararı: saf paketler workflow/lossratio; DB/HTTP task/report; audit.go değişmez; sıfır SLA seçenekleri hata verir. Kod ajanı push yapmadı.

- `make test`: PASS (58 birim test fonksiyonu).
- `./scripts/test-integration.sh`: PASS (59 entegrasyon testi + TestMain; 00006–00008 dahil Up/Down/Up).
- `./scripts/test-integration.sh -run TestConcurrent -count=10 -race`: PASS; mevcut sekiz yarış testinin her biri 10 tekrar.
- `make up`, `make verify` (geri yüklenen kaynakla): PASS; db/api/web healthy, 47 PASS / 0 FAIL.
- İlk Docker derlemesi geçici kilit mutasyonuyla çakıştı; o imajdaki verify 46 PASS / 1 FAIL (201/409). Kaynak geri konulduktan sonra imaj yeniden üretildi, canlı verify 47/0 geçti. Mutasyon imajı kullanılmıyor.
- İade mutasyonu: poliçe kilidi geçici kaldırıldı; `./scripts/test-integration.sh -run TestConcurrentRefunds -count=1 -v` beklenen FAIL, 201 + 409 CONCURRENT_PAYMENT. installments CHECK paid>=0 savunması yakaladı; fazla iade commit olmadı.
- Mutasyon commit edilmedi. `git diff -- backend/internal/payment/service.go` geri yüklemeden sonra **boş çıktı** (0 bayt); `/tmp/gp-a6-refund-restored.diff`. audit.go, saf billing.Adjust ve 00001–00005 değişmedi.

```text
$ git diff -- backend/internal/payment/service.go
(çıktı yok)
$ wc -c /tmp/gp-a6-refund-restored.diff
0 /tmp/gp-a6-refund-restored.diff
```

Değişen eski beklentiler: seed B sabit 300000 yerine gerçek tarifeden 1422222 kuruş; 20 günlük ilk taksit faizi 2000 yerine 9481. İptal sonrası test ayar öncesi 100000 anaparaya 15 gün için 1500 kuruşu doğrular. CLOSED reddedilmiş hasar koruma testi genişletildi. Negatif zeyilde taksit, açık bakiye ve nakit iade tutarları artık ayrı doğrulanır.

Mevcut tarihsel seed sürümleri/yevmiye kayıtları yerinde değiştirilmedi; düzeltilmiş seed temiz geçici veritabanında doğrulandı. Eski demo kaydı korunur. Geriye dönük iptalden sonra, etki tarihinden ilerisine daha önce tahakkuk etmiş faiz geri alınmaz (ADR-15'te ayrıca kaydedilecek).

### 0. bölüm yerel commitleri

- `d5574a9 docs: A6 görev tanımı`
- `16c7a6f fix(billing): geriye dönük ek prim vadesini bugünden önceye alma`
- `62fdb8a fix(billing): prim ayarından önce ortak oranla faiz tahakkuk ettir`
- `934ee33 fix(ledger): prim ayarı ve nakit iadeyi işlem tarihinde kaydet`
- `f0173b1 fix(policy): reddedilip kapanmış hasarı değişiklik engeline dahil etme`
- `67ac562 fix(idempotency): iş anahtarıyla çökme tekrarını önle ve yanıt baytlarını koru`
- `d8edffe fix(db): zeyil bağlantısını aynı poliçe ve üretilen sürümle sınırla`
- `24f6467 fix(claim): incelemeden ekspertize dönüşü ve ekspertiz koşulunu ekle`
- `ab80483 fix(seed): primleri tarifeden hesapla ve demo uzman atölye havuzunu tamamla`
- `fa5dbec fix(payment): tarih sınırı, kilitsiz okumalar ve iade yarışını doğrula`
- `252ea30 docs(progress): geçmiş commit hashlerini mevcut mesajlarla eşleştir`


### A6 teslim raporu — AGENTS.md §6

1. **Durum: TAMAMLANDI.** 0. bölüm kapısından sonra durulmadı; T-5 ve F-7 backend, seed, minimal frontend ve gerçek verify tamamlandı. Süre nedeniyle kapsam kesilmedi. A7'ye geçilmedi; push yapılmadı.
2. **Değişen dosyalar:** tam yollar ve tek satırlık amaçlar aşağıdaki manifestte.
3. **Yapılanlar:** Saf fraud/workflow/lossratio; DB/HTTP task/report paketleri. Bildirimde bir kez fraud, NULL otomatik geçmiş aktörü ve audit.New.auto; Transition görev yaşam döngüsü; rol kilitli en az yük ataması; enjekte saatli SLA, DB hatasına dayanıklı scrape; defterden dönem/boyut raporu. FP-1/FP-2 servis seed'i idempotenttir. Sıfır TransitionOptions SLA'sı hata verir. audit.go ve imzası, 00001–00005 ve saf billing.Adjust değişmedi. Genel A4 refactor yapılmadı; uzun A6 sorguları çok satırlı raw string'dir.
4. **Testler (son kaynak):**
   - `make test` — PASS; 66 birim test fonksiyonu.
   - `./scripts/test-integration.sh` — PASS; 67 entegrasyon testi + TestMain; gerçek PostgreSQL 16, app 51,996 sn. Migrasyon Up/Down/Up dahil.
   - `./scripts/test-integration.sh -run TestConcurrent -count=10 -race` — 0. bölüm kapısında PASS; mevcut sekiz yarış testi.
   - `./scripts/test-integration.sh -run 'TestConcurrentTaskAssignment|TestFraudSeedStable' -count=10 -race -v` — PASS (296,007 sn); 20 bağımsız transaction kapıda, her tekrarda 7/7/6; seed/metrics hata testi de on tekrar.
   - `go test ./internal/fraud ./internal/workflow ./internal/lossratio -coverprofile=/tmp/gp-a6-pure.cover` (backend dizininde) — her paket %100 PASS.
   - `go test ./internal/auth -run '^TestTokenTamperedSignature$' -count=1000` (backend dizininde) — PASS; ayrı onaylı test düzeltmesi.
   - `make lint build` — PASS; normal/integration Go vet, ESLint, Go ve TypeScript/Vite build.
   - `make up`, `make ps` — PASS; db/api/web healthy.
   - `make verify` — **59 PASS / 0 FAIL**; F-7 dört, T-5 yedi ve T-4 idem-bayt dahil.
   - OpenAPI 0.6.0 YAML ve 265 yerel $ref hedefi Ruby YAML ile doğrulandı; 37 path, 48 schema.
   - CUA 375×812: görev/ihlal/fraud/eksper/rapor, boş/hata ve toplam; scrollWidth=innerWidth=375. Viewport geri alındı, sekme kapandı. **ÇALIŞTIRILMADI:** sarı son-%25 rozeti için ayrıca gerçek verili görsel senaryo; koşul uygulandı. Görseller /tmp/gp-a6-ui-{tasks,report,appraiser}-375.png.
5. **Bilinen eksikler / riskler:** Kurgusal model; yeniden fraud skoru, manuel atama/mesai takvimi, kazanılmış prim/muallak/IBNR yok. Farklı poliçelerde READ COMMITTED sayımı ±1 olabilir. Geçmişe etkili iptalde önceden tahakkuk etmiş sonraki faiz geri alınmaz. Önceden JSONB saklanmış yanıtların eski özgün HTTP baytları geri kazanılamaz; yeni normal yanıtlar bytea ile aynıdır. Eski seed mali kayıtları korunur. Gün sonu ölçek/hata devamı A7, genel okunabilirlik A8. Yeni migrasyonlar 00006–00008; geçiş geçmişi varsa 00008 Down, sıfır toplam varsa eski 00005 Down başarısız olabilir; veri silinmez.
6. **Atılan commit'ler:** aşağıda hash + mesaj listesi; son teslim doküman commit'i `git log cca6559..HEAD` ve kullanıcıya son yanıtta yer alır. Push yok.
7. **Sonraki görev:** yalnız öneri A7; yeni CURRENT_TASK/onay olmadan başlanmaz.

**Ek onay ve ara sonuçlar:** Eski TestTokenTamperedSignature son tam koşuda FAIL verdi: son Base64 karakterinin A→B değişimi bazen aynı imza baytlarıydı. Kullanıcı yalnız test düzeltmesini ayrıca onayladı; ilk imza karakteri değiştirildi. Üretim auth kodu değişmedi; 1000 tekrar ve tam paket geçti. ad8fb22 commit'i test sonucunu okumadan yanlışlıkla atıldı; MaxInt64 payda sınırı FAIL idi, hemen b8bb4c5 ile düzeltildi. Geçmiş yeniden yazılmadı; son kaynak testleri yeşil. 0. bölümdeki geçici mutasyon imajı/ilk verify hatası yukarıda kayıtlıdır; son imaj doğru kaynakla yeniden üretildi.

**Mutasyon geri alma kanıtı:** Advisory kilitsiz atama beklenen FAIL verdi (20 bekleyen yok; 5/7/8). İade mutasyonu 201/409 CHECK savunmasıyla FAIL verdi. Mutasyonlar commit edilmedi.

```text
$ git diff -- backend/internal/task/store.go
(çıktı yok)
$ git diff -- backend/internal/payment/service.go
(çıktı yok)
$ wc -c /tmp/gp-a6-assignment-restored.diff /tmp/gp-a6-refund-restored.diff
0 /tmp/gp-a6-assignment-restored.diff
0 /tmp/gp-a6-refund-restored.diff
0 total
$ git diff cca6559 -- backend/internal/audit/audit.go db/migrations/0000[1-5]*
(çıktı yok)
```

### A6 dosya manifesti

| Tam yol | Amaç |
|---|---|
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/.env.example](</Users/macair/Okulda yapılan çalışma/guvenpolice/.env.example>) | Kurgusal fraud puan/eşik ve SLA env ayarları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/CURRENT_TASK.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/CURRENT_TASK.md>) | Kullanıcının onaylı A6 tanımı; ilk commit yalnız bu dosya. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/DECISIONS.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/DECISIONS.md>) | ADR-14–18 kararları ve açık sınırlamalar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/PROGRESS.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/PROGRESS.md>) | Eski hash eşlemesi, 0. bölüm kapısı ve kanıta dayalı teslim raporu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/ROADMAP.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/ROADMAP.md>) | Yalnız A6 kutuları ve F-7/T-5 kanıt matrisi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/TESTING.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/TESTING.md>) | Kabul komutları/sonuçları, seed/UI ve mutasyon kanıtları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/cmd/api/main.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/cmd/api/main.go>) | Doğrulanmış fraud/SLA ayarlarını router ve seed hizmetine taşır. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_integration_test.go>) | Mevcut hasar testlerinde bağımsız geçerli plaka fixture. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/corrections_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/corrections_integration_test.go>) | 0. bölüm finans/tarih/seed/tetikleyici/çıkmaz regresyonları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/idempotency_recovery_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/idempotency_recovery_integration_test.go>) | Beş yolun çökme kurtarması, bayt eşitliği ve fraud/görev yan etkileri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/integration_test.go>) | Migrasyon sürümü 8 ve ortak kurulum doğrulaması. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/payment_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/payment_integration_test.go>) | İade gate yarışı, faiz/nakit iade/plan tutarları ve fixture plakaları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/policy_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/policy_integration_test.go>) | CLOSED reddedilmiş hasar sonrası geriye dönük değişiklik regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/report_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/report_integration_test.go>) | İki aylık altın defter verisi, dönem/gruplama/RBAC testleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/router.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/router.go>) | Görev/rapor rotaları, config ve scrape toplayıcısı bağlantıları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/workflow_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/workflow_integration_test.go>) | Fraud kalıcılığı, görev/SLA/RBAC, 20 eşzamanlı atama, seed/metrics. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/auth/jwt_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/auth/jwt_test.go>) | Ayrı kullanıcı onayıyla yalnız imza bozma testinde anlamlı bit değişimi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/fraud.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/fraud.go>) | Hasar tarihli pencere sorgusu, bir defalık sinyal kaydı ve detay okuması. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/repo.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/repo.go>) | Role bağlı fraud/tasks/allowed_transitions alanları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/service.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/service.go>) | Bildirim kurtarması, otomatik inceleme ve Appraise görev kapanışı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/settlement.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/settlement.go>) | Onay/ödemede TransitionOptions ile saat/SLA taşır. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/transition.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/transition.go>) | Tek geçiş noktasında seçenek kontrolü, otomatik geçmiş ve görev akışı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/transition_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/transition_test.go>) | Sıfır/eksik SLA seçeneklerinin DB erişiminden önce reddi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/config/config.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/config/config.go>) | Fraud/SLA env ayarlarını okur ve doğrular. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/config/config_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/config/config_test.go>) | Varsayılan/özel/sıfır/negatif/bozuk env testleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/fraud/fraud.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/fraud/fraud.go>) | DB/saat bağımsız üç kurgusal fraud kuralı ve güvenli ayarlar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/fraud/fraud_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/fraud/fraud_test.go>) | DB/saat bağımsız üç kurgusal fraud kuralı ve güvenli ayarlar için altın/sınır testleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/idempotency/context.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/idempotency/context.go>) | Context iş anahtarı ve named UNIQUE ihlalini 409 eşleme. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/idempotency/idempotency.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/idempotency/idempotency.go>) | Normal yanıtı bytea baytlarıyla saklar ve tekrar döner. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/lossratio/lossratio.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/lossratio/lossratio.go>) | big.Int ile güvenli yarımı yukarı baz puan oranı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/lossratio/lossratio_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/lossratio/lossratio_test.go>) | big.Int ile güvenli yarımı yukarı baz puan oranı için altın/sınır testleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/repo.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/repo.go>) | GET billing kilitsiz tek REPEATABLE READ READ ONLY snapshot. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/run.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/run.go>) | Ortak faize doğru dönem/yevmiye tarihlerini geçirir. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/service.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/service.go>) | Poliçe kilidinden sonra erken iş anahtarı; ödeme tarihi alt sınırı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/accrual.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/accrual.go>) | Faiz aralığı/yevmiye tarihini ayırır; tek oran kaynağı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/adjustment.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/adjustment.go>) | Ek prim vadesi, ayar öncesi faiz, işlem tarihli mali kayıt/iade. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/payment.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/payment.go>) | Manuel iş anahtarı ve işlem tarihli otomatik nakit iade. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/replay.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/replay.go>) | Aktör/poliçe/anahtar üzerinden mevcut ödemeyi okur. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/platform/metrics/metrics.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/platform/metrics/metrics.go>) | Mutex dışında 250 ms timeoutlu scrape toplayıcısı; hata izolasyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/platform/metrics/metrics_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/platform/metrics/metrics_test.go>) | Taze scrape, hatada eski değer saklamama ve kilitsiz callback. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/change.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/change.go>) | Zeyil/iptal kurtarması, saat/oran, kilitsiz önizleme ve red istisnası. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/handler.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/handler.go>) | Ortak saat/faiz oranını değişiklik hizmetine geçirir. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/replay.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/replay.go>) | Poliçe kilidi sonrası zeyil/iptal iş anahtarını erken bulur. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/report/report.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/report/report.go>) | 4000/5000 defterden dönem/boyut raporu ve HTTP doğrulaması. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/billing.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/billing.go>) | A/B poliçelerini geçerli plakayla tarifeden fiyatlar; eskiyi korur. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/demo.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/demo.go>) | İkinci eksper/uzman, üç atölye ve commit sonrası servis seed akışı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/fraud.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/fraud.go>) | FP-1/FP-2 numara/iş anahtarlarıyla fraud/SLA servis senaryosu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/policy.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/policy.go>) | Gerçek tarife fiyat motoru ve mevcut Issue yardımcısını kullanır. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/task/handler.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/task/handler.go>) | Görev filtre/keyset, zorunlu eksper ataması ve özet/iş yükü HTTP. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/task/repo.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/task/repo.go>) | Görev/iş yükü/SLA sorguları ve enjekte saatli yanıtlar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/task/store.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/task/store.go>) | Aynı transactionda görev aç/kapat; rol advisory kilitli atama. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/workflow/workflow.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/workflow/workflow.go>) | Saf vade/ihlal/atama/boş havuz kuralları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/workflow/workflow_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/workflow/workflow_test.go>) | Saf vade/ihlal/atama/boş havuz kuralları için altın/sınır testleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00006_business_request_keys.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00006_business_request_keys.sql>) | İş anahtarları/kısmi UNIQUE indeksler ve JSONB→bytea. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00007_policy_endorsement_link_guard.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00007_policy_endorsement_link_guard.sql>) | Zeyil bağını aynı poliçe ve ürettiği sürümle sınırlar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00008_claim_workflow_indexes.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00008_claim_workflow_indexes.sql>) | Fraud→Appraisal geçişi; eksik workshop/yevmiye tarih indeksleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docker-compose.yml](</Users/macair/Okulda yapılan çalışma/guvenpolice/docker-compose.yml>) | Fraud/SLA env ayarlarını API konteynerine aktarır. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/api/types.ts](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/api/types.ts>) | Fraud, görev, iş yükü ve rapor yanıt tipleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/ClaimDetail.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/ClaimDetail.tsx>) | Fraud açıklamaları/görevler ve sunucu geçiş butonları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/TaskTable.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/TaskTable.tsx>) | Ortak görev tablosu ve kırmızı/sarı/yeşil SLA rozeti. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/ClaimsPage.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/ClaimsPage.tsx>) | FRAUD_REVIEW durum filtresi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/Dashboard.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/Dashboard.tsx>) | Rol korumalı görev/fraud/rapor ekranlarını bağlar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/ReportsPage.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/ReportsPage.tsx>) | Dönem/grup seçimi, TL/yüzde/toplam ve boş/yükleniyor/hata. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/TasksPage.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/TasksPage.tsx>) | Görevlerim, görev/fraud filtresi ve uzman iş yükü paneli. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/roles.ts](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/roles.ts>) | Görev/fraud ve muhasebe/UW rapor menüleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/styles.css](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/styles.css>) | SLA renkleri ve 375 px görev/rapor satırları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/openapi.yaml](</Users/macair/Okulda yapılan çalışma/guvenpolice/openapi.yaml>) | 0.6.0 uç/alan/hata sözleşmesi ve dönem tanımı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/requests.http](</Users/macair/Okulda yapılan çalışma/guvenpolice/requests.http>) | Mevcut named login değişkenleriyle A6 istek örnekleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/verify.sh](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/verify.sh>) | F-7/T-5, paralel farklı poliçe ataması ve ödeme cmp kanıtları. |

### A6 yerel commitleri (teslim dokümanından önce)

- `d5574a9 docs: A6 görev tanımı`
- `16c7a6f fix(billing): geriye dönük ek prim vadesini bugünden önceye alma`
- `62fdb8a fix(billing): prim ayarından önce ortak oranla faiz tahakkuk ettir`
- `934ee33 fix(ledger): prim ayarı ve nakit iadeyi işlem tarihinde kaydet`
- `f0173b1 fix(policy): reddedilip kapanmış hasarı değişiklik engeline dahil etme`
- `67ac562 fix(idempotency): iş anahtarıyla çökme tekrarını önle ve yanıt baytlarını koru`
- `d8edffe fix(db): zeyil bağlantısını aynı poliçe ve üretilen sürümle sınırla`
- `24f6467 fix(claim): incelemeden ekspertize dönüşü ve ekspertiz koşulunu ekle`
- `ab80483 fix(seed): primleri tarifeden hesapla ve demo uzman atölye havuzunu tamamla`
- `fa5dbec fix(payment): tarih sınırı, kilitsiz okumalar ve iade yarışını doğrula`
- `252ea30 docs(progress): geçmiş commit hashlerini mevcut mesajlarla eşleştir`
- `c230bfb docs: A6 düzeltme kapısı ve iade mutasyonu kanıtlarını kaydet`
- `ad8fb22 feat(domain): saf fraud, workflow SLA ve lossratio hesapları`
- `b8bb4c5 fix(lossratio): büyük paydada yuvarlama ara taşmasını önle`
- `84b9690 feat(task): rol kilitli atama, SLA sorguları ve scrape toplayıcısı`
- `e92d851 feat(claim): bildirimde fraud skoru, otomatik inceleme ve SLA görev akışı`
- `f4d987f feat(report): defterden dönem ve boyut bazlı prim hasar raporu`
- `237f8ee feat(seed): servislerle idempotent fraud ve gecikmiş SLA örnekleri`
- `41182ad test(auth): imza mutasyonunda anlamlı Base64 bitlerini değiştir`
- `7799b22 fix(domain): fraud ayar taşmasını önle ve sıfır gün detayını koru`
- `b8ca8fc test(task): seed sabitliğini, sayısal SLA ve kilit mutasyonunu doğrula`
- `4e955a6 feat(web): fraud kuyruğu, SLA görevleri ve prim raporu ekranları`
- `5c12355 test(verify): F-7 rapor ve T-5 paralel atama kanıtlarını ekle`

## A7 (ajan: Codex)

Başlangıç: 2026-10-25 22:13 TRT. Kapsam onaylı A7; push ve A8 yok. Kullanıcının ek koşulları: yalnız seed/simülatör bağlam saati ayarlar; API gerçek zaman regresyon testi; simülatör oturumlarında synchronous_commit=off; 1000 ölçümünden gerekirse worker artırımı ve arka planda 100k; yarım koşu kurtarması reset-demo + simulate.

### 0. bölüm — gün sonu

İmleçli aday taraması, varsayılan 500 / en çok 2000, gerçek saatle 8 saniyelik başlangıç bütçesi, ayrı transaction, kilitli ve başarısız poliçede imleç ilerlemesi ve failed listesi eklendi. UI ve verify done olana kadar devam eder. Başlangıç şemasında vade indeksi bulunduğundan tekrar eklenmedi; 00009 açık faiz ve audit occurred_at indeksini, simulation_runs tablosunu ve kurgusal v0 tarifelerini ekler. Mevcut v1/v2 değişmez. Migration/seed tarihsel tarifeleri reset sonrasında aynı veriyi üretir.

İlk tam entegrasyonda iki eski 2024/NO_TARIFF beklentisi yeni v0 kapsamına girdiği için kırıldı. Testler 2024'te v0'ı doğrulayacak ve tarife-yok yolunu 0001-01-01 öncesinde koruyacak şekilde güncellendi. İlk hedefli gün sonu, migration Up/Down/Up ve reset sonrası tarife eşitliği testleri PASS. Gerçek API verify ilk koşusu: 60 PASS, 0 FAIL.

0. bölüm kapısı PASS: `make test`; `make test-integration` (saat bağlantısını da içeren tekrar); `./scripts/test-integration.sh -run 'TestConcurrent' -count=10 -race -v` (9 test ×10, app 371.142 s); `make lint build`; `make up` (db/api/web healthy); `make verify` (iki koşu, her biri 60 PASS / 0 FAIL). Ayrı hedefli testler üç parça/tek sonuç, hata yalıtımı+rollback, imleç, gelecekteki vade, yalnız açık faiz, API gerçek timestamp, migration Up/Down/Up ve reset sonrası v0 eşitliğini doğruladı. Loglar `/tmp/gp-a7-*-gate.log`, `/tmp/gp-a7-history.log`, `/tmp/gp-a7-clock-tests.log`, `/tmp/gp-a7-dayend-extra.log`.

Kapı commitleri: 0ce7933 (görev metni), 57b2452 (gün sonu backend), aa3157d (UI/verify döngüsü), d06890b (00009/v0), 0e9d43c (v0 test sınırı), ac8d28c (bağlam saati/API-now), ab142f2 (açık faiz ve doğrulama). Kapı sonrası simülatör fazlarına devam edilir.
