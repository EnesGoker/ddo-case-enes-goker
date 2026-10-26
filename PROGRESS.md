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


### Docker disk kesintisi ve çevrimdışı çalışma

2026-10-25 22:36 TRT: ilk 1000 poliçe entegrasyon denemesi 51.921331334 s sonra `SIM-000855: conn closed` ile kesildi. Bu **tamamlanmış 1000 ölçümü değildir** ve 100k süre kanıtı sayılmaz. Docker VM/host günlükleri `no space left on device`, EXT4 I/O hataları ve motorun kapandığını doğruladı. Kaynak: Docker `Data/log/host/com.docker.backend.log`, 19:36:22 UTC. Docker/API sağlığı ve ana DB mutabakatı geri dönüşte tekrar kontrol edilecek; ana veri sıfırlanmadı.

Kullanıcı yalnız `go clean -cache` için onay verdi; komut başarıyla çalıştı (önbellek ~1 GB → 8 KB). Kullanıcı ana disk alanını kendisi açacak ve Docker'ı kendisi başlatıp haber verecek. **Haber gelmeden Docker başlatılmaz ve başka temizlik/silme yapılmaz.** Bekleyen iki test komutu, Docker'a yeniden işlem göndermemeleri için durduruldu; kalan geçici test konteynerlerine ayrıca dokunulmadı.

Çevrimdışı gerçek sonuçlar:
- `make test`: PASS (`/tmp/gp-a7-offline-unit.log`).
- `cd backend && go test -race ./internal/platform/clock ./internal/simulation ./internal/payment`: PASS (1.888 / 4.687 / 4.634 s; `/tmp/gp-a7-offline-race.log`).
- `cd backend && go vet -tags integration ./...`: PASS.
- `cd backend && go test -tags integration -c -o /tmp/gp-a7-integration.test ./internal/app`: PASS; bu derleme, testleri çalıştırmaz.
- `cd frontend && npm run lint && npm run build`: PASS (`/tmp/gp-a7-offline-web.log`).
- `bash -n scripts/verify.sh scripts/reset-demo.sh scripts/test-integration.sh`, `git diff --check`: PASS.

Bu kesinti anında simülatörün DB orkestrasyonu/aktivite/UI kodu gerçek DB kabulünden geçmediği için commit edilmedi; sonraki kabul/commit bilgisi aşağıdadır. Yeni DB testleri, 1000 × üç taze DB/parmak izi/worker kıyası, 100k gerçek ölçek, süreli verify, canlı mod ve 375 px altı rol kabulü Docker sonrasına bekliyor. Hedef 100000 olarak kalır. Ek YAML kontrolü ilk js-yaml denemesinde paket bulunamadığı için çalışmadı. Ardından repoda zaten bulunan `go.yaml.in/yaml/v3` ile `GOPROXY=off go run /tmp/gp-a7-checkyaml.go ../openapi.yaml ../docker-compose.yml` çalıştırıldı: iki YAML da PASS; yeni bağımlılık kurulmadı.


Çevrimdışı son ek kontroller: `go test ./...` PASS (`/tmp/gp-a7-offline-unit-final.log`); frontend lint/build tekrar PASS (`/tmp/gp-a7-offline-web-final.log`). Saf 100000 plan dağılımı testi PASS: KASKO 5447 / 16169079 maruz gün → 1229 bp; TRAFIK 1934 / 11428983 → 617 bp; KONUT 535 / 4895319 → 398 bp. Bu yalnız plan hesabıdır, gerçek DB üretimi veya ölçek ölçümü değildir. Kasko'nun ~%12 toplamında üçlü şüpheli örnekler için normal bildirim sıklığından pay ayrılır.

Bağımsız parametre yükleme/hash testleri ayrıca yalnız `config.go config_test.go` dosyalarıyla PASS olduğu için simülatörün bu temeli ayrı commit edildi. Orkestrasyon/DB, Docker, aktivite, UI ve kabul testleri hâlâ gerçek DB kabulünü bekliyor. Canlı modun normal sonlandırması parmak izini temizler; özetin batch sonuna ait olduğu UI ve belgelerde belirtilir. Gerçek saat bütçesi ve bağlantı-hatası ayrımı için ilave regresyonlar yazıldı; bütçe DB testi henüz çalışmadı.


### Docker sonrası kabul ve 100k başlangıcı

Kullanıcı 2026-10-25 23:03 TRT civarında alan açıldığını ve Docker'ın çalıştığını bildirdi. Yeniden başlatma kullanıcı tarafından yapıldı. Başka disk temizliği yapılmadı. Ana API/DB/web healthy; gerçek API mizanda `balanced=true`, satırsız/dengesiz 0, 1200=42781506; açık anapara 42708973 + faiz 72533; mutabakat true.

`./scripts/test-integration.sh -run TestSimulatorDeterminismAndInvariants -v -timeout 20m`: PASS (app 212.448 s). Taze DB ölçümleri: seed42/8worker 80.432537625 s → doğrusal 2:14:03; seed42/16worker 55.143650958 s → 1:31:54; seed43/8worker 56.079574 s. İlk 8-worker koşusunda Docker imaj derlemesi de çalışıyordu; bunlar izole performans kıyası değildir. 8/16 parmak izi aynı: fc51238e1b71621e266b6d1c07cdba70b015c368bd19e2212023b1531c97e411. Seed43: 7b9ebdbc4215948fad3968b65d79ed5cd7bebefad24ec469bebfe75ed1104127. Tekrar koşu veri eklemedi; mali/izolasyon/geçiş kontrolleri, koşu kilidi ve canlı modun hasar üretmemesi/sonlandırılması PASS.

`./scripts/test-integration.sh -run 'TestActivity|TestBillingRunUsesRealTimeBudget|TestAPICreatedTimestampsUseRealNow' -v`: PASS. `./scripts/test-integration.sh -run TestSimulatorResetRemovesDatasetAndRestoresTariffs -v`: PASS (9.59 s); yalnız taze test DB'sinde reset whitelist'i simülasyon ve koşu kaydını temizledi, seed 3 v0 tarifesini geri kurdu. Ana DB reset edilmedi.

16 worker tahmini hâlâ 60 dakikayı aştığı için kullanıcı talimatıyla 100k arka planda başlatıldı: `SIM_ANCHOR_DATE=2026-10-25 SIM_SEED=42 SIM_POLICIES=100000 SIM_YEARS=5 SIM_WORKERS=16 /usr/bin/time -p make simulate`. Başlangıç 23:10 TRT; log `/tmp/gp-a7-sim-100k.log`. Gerçek tamamlanma süresi/DB boyutu henüz ölçülmedi; verify koşu bitince çalıştırılacak. Güncel imaj nonroot:nonroot; varsayılan API/DB/web healthy.

### 100k sırasında onaylı çevrimiçi indeks düzeltmesi (00010)

7.000 poliçede payments için pg_stat_user_tables.seq_tup_read=17.271.335.615 ölçüldü. Billing sorgusu her ödeme satırında refund_of_payment_id üzerinden iade toplamı ararken tam tablo tarıyordu. Kullanıcı 00010 ve migration testine açık onay verdi; ardından NO TRANSACTION + CREATE INDEX CONCURRENTLY IF NOT EXISTS, aynı sorgunun önce/sonra EXPLAIN (ANALYZE) kanıtı ve koşuda ortaya çıkan benzer eksik indekslerin aynı kalıpla eklenmesini yetkilendirdi. Önceden kapsam dışı olan bu EXPLAIN ölçümü yeni talimatla yapıldı; README A8'de kalır.

00010 yalnız payments(refund_of_payment_id) WHERE refund_of_payment_id IS NOT NULL indeksidir; servis sorgusu ve 00001–00009 değişmedi. Up/Down/Up ve mevcut indeks varken goose Up tekrarı taze test DB'sinde PASS: ./scripts/test-integration.sh -run TestMigrationsUpDownUp -v. Ana DB'ye mevcut goose bağımlılığıyla geçici statik migration yardımcısı üzerinden uygulandı; API ve simülatör durdurulmadı. Goose 20:23:50 UTC'de sürüm10'u 848,89 ms'de uyguladı. Kalıcı binary/kod bağımlılığı eklenmedi.

Aynı sorgu, aynı policy_id=54 (SIM-000020), aynı 11 ödeme, PostgreSQL16; ilk ölçümde tabloda 72.560 ödeme. Ölçümler devam eden 16-worker koşusu altında tek örnektir; izole yük testi veya soğuk cache kıyası değildir. Execution Time 310,638 ms → 0,240 ms, shared hit 13.085 → 30; alt sorgu Seq Scan → payments_refund_of_payment_idx Index Scan. İki ham çıktı ve sorgu aşağıdadır; A8 README karşılaştırmasına gerçek kanıt olarak aktarılabilir.

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT p.id, p.policy_id, p.kind, p.amount_kurus, p.method, p.refund_of_payment_id,
       p.paid_at, p.journal_entry_id, coalesce(p.reference,''),
       CASE WHEN p.kind = 'PAYMENT' THEN p.amount_kurus - coalesce((
         SELECT sum(r.amount_kurus) FROM payments r WHERE r.refund_of_payment_id = p.id
       ),0) ELSE 0 END
FROM payments p WHERE p.policy_id = 54 ORDER BY p.paid_at DESC, p.id DESC;
```

Önce:
```text
                                                                        QUERY PLAN
----------------------------------------------------------------------------------------------------------------------------------------------------------
 Incremental Sort  (cost=2102.28..23122.52 rows=11 width=127) (actual time=310.584..310.585 rows=11 loops=1)
   Sort Key: p.paid_at DESC, p.id DESC
   Presorted Key: p.paid_at
   Full-sort Groups: 1  Sort Method: quicksort  Average Memory: 26kB  Peak Memory: 26kB
   Buffers: shared hit=13085
   ->  Index Scan Backward using payments_policy_idx on payments p  (cost=0.29..23122.02 rows=11 width=127) (actual time=43.930..310.515 rows=11 loops=1)
         Index Cond: (policy_id = 54)
         Buffers: shared hit=13076
         SubPlan 1
           ->  Aggregate  (cost=2099.59..2099.60 rows=1 width=32) (actual time=28.217..28.217 rows=1 loops=11)
                 Buffers: shared hit=13066
                 ->  Seq Scan on payments r  (cost=0.00..2099.59 rows=1 width=8) (actual time=26.598..26.598 rows=0 loops=11)
                       Filter: (refund_of_payment_id = p.id)
                       Rows Removed by Filter: 72560
                       Buffers: shared hit=13066
 Planning:
   Buffers: shared hit=156
 Planning Time: 3.507 ms
 Execution Time: 310.638 ms
(19 rows)


```

Sonra:
```text
                                                                           QUERY PLAN
----------------------------------------------------------------------------------------------------------------------------------------------------------------
 Incremental Sort  (cost=10.85..116.80 rows=11 width=127) (actual time=0.188..0.189 rows=11 loops=1)
   Sort Key: p.paid_at DESC, p.id DESC
   Presorted Key: p.paid_at
   Full-sort Groups: 1  Sort Method: quicksort  Average Memory: 26kB  Peak Memory: 26kB
   Buffers: shared hit=30
   ->  Index Scan Backward using payments_policy_idx on payments p  (cost=0.29..116.31 rows=11 width=127) (actual time=0.127..0.139 rows=11 loops=1)
         Index Cond: (policy_id = 54)
         Buffers: shared hit=21
         SubPlan 1
           ->  Aggregate  (cost=8.17..8.18 rows=1 width=32) (actual time=0.003..0.003 rows=1 loops=11)
                 Buffers: shared hit=11
                 ->  Index Scan using payments_refund_of_payment_idx on payments r  (cost=0.15..8.17 rows=1 width=8) (actual time=0.002..0.002 rows=0 loops=11)
                       Index Cond: (refund_of_payment_id = p.id)
                       Buffers: shared hit=11
 Planning:
   Buffers: shared hit=175
 Planning Time: 18.691 ms
 Execution Time: 0.240 ms
(18 rows)


```

### Rol kabulünde kullanıcı yönlendirmesi: ADJUSTMENT ön koşulu

375 px denemesinde ekspertizsiz APPRAISAL → ADJUSTMENT geçişi gerçekleşti; tazminat önizlemesi ekspertiz hatası verince GP-H-00000040 demo dosyası FRAUD_REVIEW üzerinden ekspertize döndürüldü. Bu amaçla fraud incelemesi kullanmak uygun değildi. Kullanıcı tüm ADJUSTMENT girişlerinde ekspertiz şartını ve UI düğmesinin pasifleşmesini açıkça istedi; sonraki denemelerde inceleme bu amaçla kullanılmaz. Demo denemesinin append-only geçmişi silinmedi/değiştirilmedi.

Transition imzası değişmeden, izinli geçiş kontrolünün ardından hedef ADJUSTMENT ise appraisals varlığı doğrulanır; eksikse 409 APPRAISAL_REQUIRED. Geçersiz durum çifti hâlâ INVALID_TRANSITION döner. UI ekspertiz dizisi boşken ilgili geçiş düğmesini pasif gösterir; OpenAPI açıklaması güncellendi. Yeni TestAdjustmentRequiresAppraisalOnEveryEntry dört alt durumda APPRAISAL/FRAUD_REVIEW × var/yok; yokken durum, sürüm, geçmiş, audit sayısı değişmez ve ADJUSTMENT görevi açılmaz. Mevcut eksik ekspertiz testi daha erken 409 kapısını doğrulayacak şekilde güncellendi; eski test silinmedi.

Hedefli beş entegrasyon testi PASS (app11.371 s): ./scripts/test-integration.sh -run 'TestAdjustmentRequiresAppraisalOnEveryEntry|TestClaimTransitionsValidationAndRBAC|TestClaimValidationAndLedgerRollback|TestFraudReviewCanReturnToAppraisal|TestClaimLifecycleAndIdempotency' -v. İlk yeni test denemesinde audit.entity_id'nin TEXT olduğu fixture sorgusunda sayısal parametre kullanıldığı için test hazırlığı FAIL verdi; parametre metne çevrildi, yukarıdaki son koşu geçti. Üretim audit kodu değişmedi.

Frontend npm run lint && npm run build PASS. Docker api/web imajları derlendi, sağlıklı yeniden açıldı; simülatör durdurulmadı. Tarayıcıda ekspertizsiz GP-H-00000021 için düğme disabled ve isEnabled=false; 375 px kanıtı /tmp/gp-a7-adjustment-disabled-375.png. Yeni gerçek canlı mod ön kontrol testi TestVerifyStopsBeforeBusinessRequestsDuringLive de PASS (7,90 s): bash verify çıkış2, yalnız login/activity HTTP, iş isteği yok. Tam entegrasyon paketi bu son koşul değişikliğinden sonra yeniden çalıştırılıyor.

### 100k tamamlanma ve gerçek verify

Üretici sürüm1 /30 atölye ile gerçek100k koşusu tamamlandı: başlangıç20:10:04.071778UTC, bitiş21:00:27.405314UTC; uygulama3022,497sn, dış `/usr/bin/time -p` **real3025,12sn (50dk25,12sn)**. Parametreler seed42, anchor2026-10-25,100000 poliçe,5 yıl,16 worker. Log `/tmp/gp-a7-sim-100k.log`. Aynı anda UI, test ve indeks incelemesi yürüdü; izole benchmark değildir. Parmak izi `abc70939dcba41e974d1648012141bc97015c9bb1075750aee274c6a814d8272`.

Batch sonu özeti: KASKO ACTIVE48721/CANCELLED1279; TRAFIK34054/946; KONUT14602/398. Tam100000 SIM, başlangıçlar2021-10-25…2026-10-10. Hasar7915: ADJUSTMENT11,APPRAISAL18,APPROVED20,CLOSED7221,FRAUD_REVIEW611,PAID9,REJECTED4,REPORTED8,TRIAGED13. İşaretli1900 (1289 kapalı/611 kuyrukta). Açık630/ihlalli621 görev. Taksit PAID507364 toplam/ödenen68206761307 kuruş; PENDING35857 toplam3600780877; CANCELLED3754 toplam280649529. Tahsilat930227/68377308343 kuruş, iade4757/403432711 kuruş. Defter nakit52989925755, tazminat gideri15038599856, yazılan prim71404109473 kuruş; balanced=true/reconciled=true. Bu özet batch anına aittir; sonraki verify ve demo kabul işlemleriyle canlı DB değişmiştir.

00011 denemesi kaldırıldıktan, verify'dan önce gerçek DB boyutu **2.217.606.167 bayt /2115MB**. SIM hasarlarında ADJUSTMENT'a giriş anında ekspertizsiz geçmiş sayısı0. `/usr/bin/time -p make verify`: **61PASS/0FAIL,real112,11sn**; gün sonu103 parçada done=true; SYS canlılık, her yevmiye dengesi ve1200 mutabakatı PASS. Log `/tmp/gp-a7-verify-100k.log`. Rapor API'sinde2021-10-25→2026-10-26 aralığı61 ay (2021-10…2026-10); ilk20 fraud kuyruk dosyasının18'i SIM. Rapor toplamı demo kayıtları da dahil yazılan71435005911 kuruş,100023 poliçe,tazminat15585331856 kuruş,2182bp. Salt okunur ölçek kanıtı logu `/tmp/gp-a7-scale-api-proof.json` (bu yardımcı çıktıda report_total alanı null; sayılar gerçek API araç çıktısından alınmıştır).

ADJUSTMENT düzeltmesinden sonraki tam entegrasyon PASS (app308,036sn; `/tmp/gp-a7-integration-adjustment-final.log`). `make test lint` son v2 kaynakla PASS (`/tmp/gp-a7-v2-test-lint.log`).

### 00011 indeks denemesi ve geri alma kanıtı

Yoğun100k yazımı sırasında mizan15sn API bütçesiyle504 verdi. Kullanıcının ek indeks yetkisiyle journal_lines(entry_id) INCLUDE(id,debit_kurus,credit_kurus) denendi; Up/Down/Up test edildi. Varsayılan plan eski indeksi seçmeye devam etti: önce17,25sn, sonra18,33sn; bakım sonrası19,77sn. Yalnız tanılama oturumunda random_page_cost=1.1 ile kaplama planı16,52sn; kalıcı DB/API ayarı veya servis SQL değişikliği yapılmadı. Yazım bitince aynı eski indeksli sorgu2364,543ms, gerçek API200/2670,377ms; yükün kalkması indeks kazancı sayılmadı.

00011 Down çevrimiçi21:02:35UTC'de39,1ms ile uygulandı. Yeni deneme dosyası kaldırıldı; migration testi HEAD sürüm10'a döndürüldü. Geri alma sonrası:

```text
$ git diff -- backend/internal/app/integration_test.go db/migrations/00011_journal_entry_amounts_index.sql
(çıktı yok)
$ SELECT to_regclass('journal_lines_entry_amounts_idx');
NULL
$ goose version
10
```

00011 commit edilmedi. Kalıcı ek sorgu indeksi yalnız00010. Üretim audit.go ve00001–00008 değişmedi; kod mutasyonu uygulanmadı. Normal nontruncating VACUUM/ANALYZE yapıldı, iş verisi silinmedi. Ağır yazımda mizan/API gecikmesi kalan sınırlamadır. Deneme/Down ham logları `/tmp/gp-a7-trial-*-explain.log`, `/tmp/gp-a7-migrate11-down.log`; ana veri sıfırlanmadı.

### İşaretli oranı: kural kırılımı ve onaylı üretici düzeltmesi

Kullanıcı1900/7915 (%24,01) işaretliyi gerçekçi bulmadı; kurallar/eşikler sabitken atölye çeşitliliği,1k/10k ölçümü ve <%10 istedi. Eski koşuda EARLY+WORKSHOP1300 (1197 kasıtlı), EARLY+REPEAT+WORKSHOP599 (tamamı kasıtlı), EARLY+REPEAT1 (kasıtlı). Kural katılımları EARLY1900/REPEAT600/WORKSHOP1899; kesişimler toplanmaz. **1797/1900 (%94,58) kasıtlı üçlü**,103 normal dosya. Ham çıktı `/tmp/gp-a7-fraud-breakdown-old.log`.

Atölye300'e çıkarıldı; ilk iki poliçedeki altı yakın dönem bildirim ve her1000'de bir yakın dönem/bir tarihsel üçlü korundu. Eski her200'de üçlü yoğunluğu seyreltilerek normal Kasko%10,7 ile toplam yaklaşık%12 korundu. Parametre sürümü2; eski tamamlanmış koşu yeni üretici sonucu sanılmıyor. Fraud kodu/config/eşikleri değişmedi. Saf100k plan testi PASS: Kasko5324/16172136 maruz gün=1201bp,Trafik1967/11431837=628bp,Konut546/4894980=407bp; bu gerçek DB100k koşusu değildir.

Taze DB, seed42/anchor2026-10-25/16worker gerçek ölçümü:

| Poliçe | Atölye | Hasar | İşaretli | Oran | WORKSHOP sinyali | Süre |
|---:|---:|---:|---:|---:|---:|---:|
|1000|300|85|3|%3,53|2|35,516sn|
|10000|300|810|44|%5,43|36|514,073sn|

İki örnekte de oran kesin <%10, defter balanced ve1200 reconciliation true. İşaretli dosyalarda EARLY/REPEAT/WORKSHOP katılımı3/3/2 ve44/21/36. Parmak izleri1k=`4a689b6577278eafdfedd3f44ea0463629d9fc8d0acf423bb4e0741f786983c5`,10k=`92bf44c8ad91587a6b0431fe3f15615886807ce0b39558597a1aa0d97457d75a`. Host→Docker ölçümü, paralel UI ile; performans izolasyonu yok. Determinizm yeniden PASS: anchor2026-10-26 seed42 worker8/16 aynı `f76c695050358f5941e75febb1a839fe90382d0999bc23a1cc60c436181cbb41`; seed43 farklı `7428e4b185efe22b7680b8c5a327c35022a5bc5d669adca77834fd74f4d1e78e`.

Hedefli Go testleri app723,399sn PASS; ancak çalışmakta olan kabuk betiği aynı sırada düzenlendiği için test sonrasında `ette: command not found` ile127 döndü. Komutun bütünü PASS değildir. Kaynak betikte yabancı komut yok; son kaynakla tam entegrasyon yeniden çalıştırılır. Yeni10k kabulü için varsayılan Go test bütçesi20dk; API bütçesi değişmedi.

**Sınırlama ve sabah devri:** WORKSHOP_CONCENTRATION mutlak adet eşiğidir, atölyenin kendi normal hacmine göre sapma hesaplamaz (ADR-16/19). 1k/10k yeni100k oranını kanıtlamaz. Kullanıcı talimatıyla **v2 üreticiyle100k bu gece ÇALIŞTIRILMADI**. Mevcut100k korunur; sabah A8 ölçümlerinden önce yeni imajla `make reset-demo` → aynı açık seed/anchor ile `make simulate` → `make verify` gerekir. Hedef100000 olarak kaldı; A8'e başlanmadı.

### Sarı SLA ve rol kabul verisi

Kullanıcı yalnız kabul verisine onay verdi. Geçici seed yardımcısı mevcut issuePriced/Pay/Report/ChangeStatus servisleriyle GP-DEMO-SLA-YELLOW ve GP-H-00007968(id7968) üretti. Görev60saat önce açıldı,72saat SLA; fraudscore0/flagfalse; API saati değişmedi, mevcut kayıt zamanları UPDATE edilmedi. Varsayılan seed'e eklenmedi. Yardımcı kaynak kaldırıldı; `git diff -- backend/internal/seed/a7_yellow_acceptance.go backend/cmd/a7seed/main.go` boş. İşlem logu `/tmp/gp-a7-sla-seed.log`, exit0. 375px görev ekranında12sa kaldı ve DOMclass badge sla-soon, scrollWidth=innerWidth=375: `/tmp/gp-a7-sla-yellow-375.png`.

Muhasebe rolü poliçe47'ye100000kuruş ödeme ve10000kuruş iade yaptı (935385/935386); ilk taksit net90000 ve açık bakiye1391480kuruş doğrulandı. UI gün sonu done/Tamamlandı, hata yok; bilinen INVALID_EFFECTIVE_DATE atlamaları listelendi. Ardından mizan dengede, satırsız/dengesiz0 ve1200 mutabakatı sağlandı. Defter Daha fazla kayıt ile20 ek kayıt yükledi. Diğer rol akışları ve son ekran kontrolü TESTING rol tablosundadır.

### A7 değişen dosyalar — tam yol ve amaç

| Dosya | Amaç |
|---|---|
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/.env.example](</Users/macair/Okulda yapılan çalışma/guvenpolice/.env.example>) | Simülatörün varsayılan SIM_* ayarları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/CURRENT_TASK.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/CURRENT_TASK.md>) | Kullanıcının onaylı A7 görev tanımı; ilk commit. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/DECISIONS.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/DECISIONS.md>) | ADR-19…22 ve mutlak fraud eşiği/üretici v2 kararı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/Makefile](</Users/macair/Okulda yapılan çalışma/guvenpolice/Makefile>) | simulate hedefi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/PROGRESS.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/PROGRESS.md>) | Gerçek ölçüm, hata, geri alma ve teslim kanıtları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/ROADMAP.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/ROADMAP.md>) | A7 kabul durumu ve sabaha kalan v2 100k. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/TESTING.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/TESTING.md>) | Kurtarma, test komutları, ölçümler ve altı rol tablosu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/Dockerfile](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/Dockerfile>) | Aynı nonroot imajda API ve simülatör binaryleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/cmd/simulator/main.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/cmd/simulator/main.go>) | Sinyal/parametre/DB ile simülatör giriş noktası. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/activity_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/activity_integration_test.go>) | Tüm rollerin canlılık ve DB hata kabulü. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/billing_run_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/billing_run_integration_test.go>) | Parça eşitliği, hata yalıtımı ve gerçek zaman bütçesi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_adjustment_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_adjustment_integration_test.go>) | Tüm ADJUSTMENT girişlerinde ekspertiz regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_integration_test.go>) | Mevcut eksik ekspertiz beklentisini erken 409 kapısına bağlar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/clock_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/clock_integration_test.go>) | API kayıtlarının gerçek istek zamanında oluştuğunu kanıtlar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/historical_tariff_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/historical_tariff_integration_test.go>) | v0 tarife ve reset sonrası eşitlik. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/integration_test.go>) | 00009/00010 Up/Down/Up ve mevcut indeksle tekrar Up. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/quote_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/quote_integration_test.go>) | v0 tarih kapsamı ve tarife-yok sınırı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/router.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/router.go>) | Kimliği doğrulanmış tüm rollere activity ucu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/simulation_fraud_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/simulation_fraud_integration_test.go>) | 1k/10k gerçek fraud oranı, patlama ve mali mutabakat. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/simulation_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/simulation_integration_test.go>) | Taze DB determinizmi, mali/izolasyon/canlı/reset kabulü. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/verify_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/verify_integration_test.go>) | Gerçek LIVE koşusunda verify iş isteklerinden önce durur. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/service.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/service.go>) | Ekspertiz INSERT zamanını işlem bağlamından okur. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/settlement.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/settlement.go>) | Onay/tüketim/ödeme zamanlarını işlem bağlamına bağlar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/transition.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/transition.go>) | Tüm ADJUSTMENT girişlerinde ekspertiz ön koşulu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/ledger/ledger.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/ledger/ledger.go>) | Yevmiye oluşturma zamanını işlem bağlamından okur. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/handler.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/handler.go>) | Gün sonu imleç/limit isteği ve yanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/run.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/run.go>) | İndeksli aday, parça, bütçe ve poliçe hata yalıtımı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/run_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/run_test.go>) | Sistemik hata ve limit doğrulaması. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/plan.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/plan.go>) | Plan oluşturma zamanını işlem bağlamından okur. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/platform/clock/clock.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/platform/clock/clock.go>) | Gerçek saate düşen bağlam saati yardımcısı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/platform/clock/clock_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/platform/clock/clock_test.go>) | Sıfır değer ve eşzamanlı bağlam izolasyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/change.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/change.go>) | Zeyil/iptal kayıt zamanlarını işlem bağlamına bağlar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/policy.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/policy.go>) | Poliçe/sürüm INSERT zamanlarını işlem bağlamından okur. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/fraud.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/fraud.go>) | Geçmiş seed işlemlerine açık bağlam saati. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/policy.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/policy.go>) | Seed poliçe üretim saatini başlangıca bağlar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/tariffs.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/tariffs.go>) | Reset sonrası kurgusal v0 tarifelerini geri kurar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/claims.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/claims.go>) | Sıralı hasar yaşam döngüleri ve korunmuş kasıtlı patlamalar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/config.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/config.go>) | 100k varsayılanları, doğrulama ve sürümlü parametre kimliği. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/config_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/config_test.go>) | Varsayılanlar, worker bağımsızlığı ve v2 kimliği. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/live.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/live.go>) | Gerçek saatli isteğe bağlı canlı poliçe/tahsilat. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/plan_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/plan_test.go>) | Tarih/sıklık/seyrek kasıtlı örnek saf testleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/policies.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/policies.go>) | Paralel servis üretimi ve varlık başına RNG. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/references.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/references.go>) | Ayrık referans kullanıcı/kimlikler ve 300 atölye. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/run.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/run.go>) | Koşu kaydı, kilidi, tekrar ve faz orkestrasyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/summary.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/summary.go>) | Mali özet ve iş alanlarının sıralı SHA-256 parmak izi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/system/activity.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/system/activity.go>) | Audit içeriğini açmadan sayısal canlılık ve koşu özeti. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00009_simulation_activity.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00009_simulation_activity.sql>) | simulation_runs, v0 ve açık faiz/audit indeksleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00010_payment_refund_index.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00010_payment_refund_index.sql>) | Tekrar güvenli, transaction dışı çevrimiçi iade FK indeksi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docker-compose.yml](</Users/macair/Okulda yapılan çalışma/guvenpolice/docker-compose.yml>) | İsteğe bağlı sim profili ve SIM_* aktarımı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/api/types.ts](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/api/types.ts>) | Parçalı gün sonu ve canlılık yanıt tipleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/ActivityIndicator.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/ActivityIndicator.tsx>) | Tüm roller için canlılık göstergesi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/ClaimDetail.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/ClaimDetail.tsx>) | Ekspertiz yokken ADJUSTMENT düğmesi pasif. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/PolicyDetail.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/PolicyDetail.tsx>) | Dönemi geçmiş ACTIVE poliçe etiketi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/format.ts](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/format.ts>) | Durumu değiştirmeden Süresi doldu görünüm yardımcısı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/hooks/useActivity.ts](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/hooks/useActivity.ts>) | 15 saniyelik gerçek API yoklaması ve hata durumu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/AccountingPage.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/AccountingPage.tsx>) | done olana kadar gün sonu ve ilerleme. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/Dashboard.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/Dashboard.tsx>) | Canlılık ve Sistem sayfasını rol uygulamasına bağlar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/PoliciesPage.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/PoliciesPage.tsx>) | Listeye Süresi doldu etiketini uygular. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/SystemPage.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/SystemPage.tsx>) | Dakikalık sayılar, ilerleme ve tamamlanmış özet. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/roles.ts](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/roles.ts>) | Tüm rollerin Sistem menüsü. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/styles.css](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/styles.css>) | Canlılık/grafik/özet için mobil stiller. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/openapi.yaml](</Users/macair/Okulda yapılan çalışma/guvenpolice/openapi.yaml>) | Activity, parçalı gün sonu ve ekspertiz ön koşulu sözleşmesi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/requests.http](</Users/macair/Okulda yapılan çalışma/guvenpolice/requests.http>) | A7 canlılık ve imleçli gün sonu örnekleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/reset-demo.sh](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/reset-demo.sh>) | Koşu tablosunu whitelist resetine ve simülatör durdurmaya ekler. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/test-integration.sh](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/test-integration.sh>) | 10k servis kabulü için gerçek Go test bütçesi 20dk. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/verify.sh](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/verify.sh>) | Parçalı gün sonu, LIVE ön kontrolü ve SYS doğrulaması. |

### Son v2 doğrulaması

`make test-integration`: **PASS,exit0**, app880,689sn; son betik20dk bütçesiyle tüm paketleri gerçekten çalıştırdı (`/tmp/gp-a7-final-integration-v2.log`). Önceki kabuk127 sorunu tekrarlanmadı. `docker compose build api` ve `docker compose up -d --no-deps --wait api`: PASS; API yeni v2 binary içeren imajla, db/api/web healthy. Simülatör veya reset başlatılmadı.

Paralel10k test yükü altında ek `/usr/bin/time -p make verify`: **58PASS/3FAIL,real279,62sn**, make çıkış2. T-4 defter-mutabakat isteği10sn istemci süresini aştı (durum000), F-7 defter/boyut kontrolleri de başarısız; bu koşu PASS sayılmadı. Önceki gerçek100k61/0 kabulü ayrı koşudur. Test yükü bitince aynı kod/veriyle verify tek başına tekrar çalıştırılır. Log `/tmp/gp-a7-final-verify-v2.log`.

Son tarayıcı tamamlaması: Muhasebe ürün raporunda100048 poliçe ve dengeli toplam; Sistem100000/100000,COMPLETED; eksper Sistem erişimi, kendi görev filtresi ve boş İncelemede listesi PASS. Sistem/görev/rapor genişliği375px, yatay taşma yok. Geçici viewport sıfırlandı ve kabul sekmesi kapatıldı. Kanıtlar `/tmp/gp-a7-system-100k-375.png`, `/tmp/gp-a7-accounting-balanced-375.png`; altı rolün gerçek sonuçları TESTING tablosunda.

Test yükü bittikten sonra aynı v2 imajıyla `/usr/bin/time -p make verify`: **61PASS/0FAIL,real110,74sn,exit0** (`/tmp/gp-a7-final-verify-alone.log`). T-4 ve F-7 mizan/rapor kontrollerinin tamamı,103 parçalı gün sonu ve SYS güncelliği geçti. Veri düzeltmesi veya süre artırımı yapılmadı. Bu sonuç mevcut v1/100k üzerindedir; yeni v2/100k üretimi sayılmaz. Son db/api/web healthy; çalışan simülatör/test konteyneri yok.

### A7 teslim özeti (AGENTS §6)

1. **Durum: KISMEN.** Bu gece istenen üretici düzeltmesi ve1k/10k kabulü tamam. A7'nin mevcut100k/5y kabulü gerçek ölçümle geçti; yeni v2 üreticiyle100k kabulü kullanıcı talimatıyla sabaha bırakıldı.
2. **Değişen dosyalar:** Yukarıdaki66 satırlık tam yol/amaç manifesti. Geçici kabul ve00011 deneme kaynakları teslimde yok.
3. **Yapılanlar:** Parçalı gün sonu; tarihsel servis saati; ayrık/deterministik simülatör ve canlı mod; canlılık/Sistem;6 rol375px kabulü; sarı SLA; evrensel ekspertiz kapısı;00010 iade indeksi;300 atölye ve seyrek kasıtlı örnekler. Kurallar/eşikler, audit.go ve00001–00008 değişmedi.
4. **Testler:** Aşağıdaki gerçek komut tablosu ve yukarıdaki başarısız/başarılı koşular.
5. **Eksikler/riskler:** V2/100k **ÇALIŞTIRILMADI** (kullanıcı talimatı).1k/10k <%10 sonucu100k oranını kanıtlamaz. Atölye eşiği kendi normal hacmine göre değil mutlak adettir. Ağır eşzamanlı yazma yükünde mizan/rapor süre aşımı ölçüldü; tek başına verify110,74sn. SCoff çökmede son işlemleri kaybedebilir; yarım koşu reset→yeniden üretim gerektirir. Geçici manuel demo inceleme geçmişi silinmedi; sabah reset kabul verisini de temizler.
6. **Commitler:** Aşağıdaki yerel liste; push yok. Son teslim belge commit'i kendi hash'ini içeremeyeceği için final yanıtta ayrıca verilir.
7. **Sonraki görev:** Sabah yeni imajla `make reset-demo` → `SIM_ANCHOR_DATE=2026-10-25 SIM_SEED=42 SIM_POLICIES=100000 SIM_YEARS=5 SIM_WORKERS=16 make simulate` → `make verify`; ardından yalnız yeni onaylı A8 görevi. Bu gece başlatılmadı.

| Komut | Gerçek sonuç |
|---|---|
| `make test lint` | PASS; Go birim, iki vet modu, frontend ESLint |
| `make test-integration` | PASS/exit0; app880,689sn, v2 dahil tam paket |
| `./scripts/test-integration.sh -run 'TestConcurrent' -count=10 -race -v` | PASS;0. bölümde9 test×10, app371,142sn |
| `cd backend && go test -race ./internal/platform/clock ./internal/simulation ./internal/payment` | PASS;1,888/4,687/4,634sn |
| `cd frontend && npm run lint && npm run build` | PASS |
| `./scripts/test-integration.sh -run TestMigrationsUpDownUp -v` | PASS;00009/00010, mevcut indeksle tekrar Up |
| `./scripts/test-integration.sh -run 'TestAdjustmentRequiresAppraisalOnEveryEntry|TestClaimTransitionsValidationAndRBAC|TestClaimValidationAndLedgerRollback|TestFraudReviewCanReturnToAppraisal|TestClaimLifecycleAndIdempotency' -v` | PASS; app11,371sn; tam pakette de tekrar geçti |
| `docker compose build api` / `docker compose up -d --no-deps --wait api` / `docker compose ps` | PASS; v2 imaj, db/api/web healthy |
| `/usr/bin/time -p make verify` (son, tek başına) | PASS;61/0,110,74sn,103 parça; mevcut100k üzerinde |
| `bash -n scripts/verify.sh scripts/reset-demo.sh scripts/test-integration.sh` / `git diff --check` | PASS |

### A7 yerel commit listesi (son teslim belge commit'inden önce)

- `0ce7933 docs: A7 görev tanımı`
- `57b2452 feat(billing): gün sonunu imleç ve poliçe hata yalıtımıyla parçala`
- `aa3157d feat(web): gün sonu parçalarını ilerleme göstererek tamamla`
- `d06890b feat(db): simülasyon kaydı ve tarihsel v0 tarifelerini ekle`
- `0e9d43c test(tariff): tarihsel v0 ve tarife-yok sınırını doğrula`
- `ac8d28c feat(clock): tarihsel INSERT saatini yalnız işlem bağlamından al`
- `ab142f2 test(billing): açık faiz adayını ve parça hata sonucunu doğrula`
- `6bba76d docs: A7 gün sonu kapısını ve simülatör kararlarını kaydet`
- `2be06f4 feat(sim): 100k varsayılanlarını ve deterministik iş parametrelerini tanımla`
- `d51b3ba fix(sim): worker bilgisini iş parmak izinden ayrı kaydet`
- `20a9619 feat(sim): servislerle tarihsel poliçe tahsilat ve hasar üret`
- `22df1a2 feat(system): tüm rollere sayısal canlılık ve koşu ilerlemesi sun`
- `5adb4f3 feat(web): canlılık göstergesi ve simülasyon sistem ekranını ekle`
- `5ecd4da test(billing): gerçek süre bütçesi ve sistem hatası ayrımını kanıtla`
- `e0773ba test(sim): taze DB determinizmi izolasyon canlı mod ve reset kabulünü ekle`
- `bfc1768 feat(demo): simülatör binary ve compose profilini bağla`
- `0a1b06f fix(db): iade toplamı sorgusuna çevrimiçi FK indeksi ekle`
- `1934601 fix(claim): değerlendirmeye her girişte ekspertiz şartını uygula`
- `536606b test(verify): canlı simülatörde erken durmayı ve canlılığı doğrula`
- `2e4dd37 fix(sim): atölye çeşitliliğini artır ve şüpheli örnekleri seyrelt`

## A8 (ajan: Codex)

Başlangıç26 Ekim08:40TRT; onaylı0.A/0.B düzeltmeleri, CI/yük/kaos/EXPLAIN/README/temiz kurulum. Kullanıcı Opus'un salt okunur raporundaki11 düzeltmeyi ve48saatlik açık dosya penceresini onayladı. İlk commit5130f39 docs: A8 görev tanımı. Olay başı limit ayrı plan/onay bekler, uygulanmadı.

Enes'in sabahki v2/100k koşusu gerçek DB'de COMPLETED/DONE100000/100000; süre29:05,060453, balanced=true/receivable_matches=true. Parmak izi6fe6f6cdb7b7c9f8f8c585b38f2f99f0be836a1bcf8282ed3df49eb4760adabd. Kullanıcı08:28'de bittiğini ve Docker'ın serbest olduğunu bildirdi. Bu başlangıç veri kümesi0.A-4 öncesidir;303 inceleme,330 açık/314 ihlalli görev için yeniden üretim açıkça onaylandı.

0.A:00011 üç v0 başlangıcını2020'ye daraltır, referansları/fiyatları korur; daha eski bağlı veri varsa tüm transaction hata verir. Seed aynı tarih; config2020 öncesinde başlamaz. Test ayrımı integration&&simulation, ayrı make test-simulation; verify yalnız mizan/üretim raporu30sn.

0.B1–3 ilk regresyonunda yeni görünürlük testinin fixture'ı30günlük poliçeydi, EARLY sinyali oluşmadığından beklenen otomatik inceleme oluşmadı.5günlük fixture'a düzeltildi. Test ayrıca demo bildiriminin herkese açık description alanındaki “fraud” sözcüğünü yakaladı; varsayılan seed açıklaması nötr hale getirildi (eski iş kayıtları UPDATE edilmedi; onaylı reset yeni seed'i kuracak). İlk koşu FAIL saklandı: /tmp/gp-a8-corrections-first.log. Son hedefli komut PASS: `./scripts/test-integration.sh -run 'TestV0Start|TestMigrationsUpDownUp|TestCustomerAndAgentClaimBodies|TestTaskCompletion|TestTaskLifecycle|TestHistoricalTariffs|TestTariffActiveByDate|TestQuoteValidation' -v` (app17,172sn; /tmp/gp-a8-corrections-second.log).

`make test lint build`: PASS (/tmp/gp-a8-unit-lint-build.log). Sistem geçişinde atama korunur; insan kapatınca CLAIM_TASK_TAKEN eski/yeni atananı aynı transaction'da audit'e yazar. ADJUSTMENT/FRAUD_REVIEW redleri DONE; iş yapılmadan APPRAISAL reddi CANCELLED; CANCELLED liste/özet SLA ihlaline girmez.

1k/10k, determinizm, canlı hata ve reset kabulü ayrı simülasyon etiketinde çalışıyor; bu sonuçlar henüz PASS sayılmadı.

### 0.A/0.B1–4 ve1k/10k kapısı

`GP_TEST_TAGS=integration,simulation GP_TEST_TIMEOUT=20m ./scripts/test-integration.sh -run '^(TestSimulator|TestVerifyStopsBeforeBusinessRequestsDuringLive)' -v`: **PASS,exit0**,app643,392sn (/tmp/gp-a8-simulation-acceptance.log). Anchor2026-10-25/seed42/16worker:1k85hasar/3işaretli(%3,53),2WORKSHOPsinyali,36,808sn;2açık inceleme/2görev/0ihlal.10k806hasar/40işaretli(%4,96),32WORKSHOPsinyali,445,294sn;7açık inceleme/7görev/1ihlal(%14,29). İki örnekte de pencere dışında açık görev0, DONE APPRAISAL sahibi≠eksper0, defter dengesi/1200 mutabakatı true. Parmak izleri4372fe78d91510f94bc599d47c3daa3688cf119d3ade12663450e12346f7cbdd ve6899a81df95d79ca417e1557632081b5655a3576f15ff3b448dfa00c26f804a8.

Ayrıca seed42/8–16worker parmak izi aynı a8b39fb68e977f0235c227fea489726666eb161d1d95a09e6c8fe42537767a17; seed43 farklı5bc28becc308e28b8caf8b7ce6b0e895c34ffafe991ae14f06e7307d60e52a57. Canlı adres uzayı dolmasıyla gerçek hata yolunda COMPLETED/LIVE_FAILED; aynı batch reset olmadan yeniden kullanılabilir. Canlı ön kontrol ve reset kabulü PASS. Bu testler olay limiti öncesi üretici sürüm3 içindir; asıl100k son kodla bir kez üretilecek.

### T-3 ek onayı ve süre kutusu

Kullanıcı nullable olay limitini, katalogda en az bir kurgusal değer ve gerçek T-3 olay-limiti verify satırıyla onayladı. Süre kutusu90dk. Başlangıç26Ekim09:13TRT civarı; kesin UTC başlangıcı aşağıdaki çalışma kaydında tutulacak. Yeni sıra: kapı kaydedildi → T-3 →0.B5–11/CI/betikler → tek100k → ölçümler.100k sırasında yalnız dosya düzenlenecek. Geçmiş kayıtlar NULL; eski settlement/defter değiştirilmez.

### T-3 tamamlanma (süre kutusu içinde)

UTCbaşlangıç06:13:16; tam DB sonucunun alındığı06:24:19 (yaklaşık11dk).00012 nullable/pozitif event_limit_kurus ekler; eski sürümlerNULL, non-NULL sözleşme varsa Down reddedilir. Katalog CAM olay başına1.000.000kuruş (10.000TL), dönem limiti2.500.000kuruş. Teklif seçimi/Issue/zeyil/sürüm okuması aynı snapshot değerini taşır. Hesap muafiyet→olay→teminat→yıllık; NULL olayı skipped; yeni calc_steps7adım. Eski settlement/audit/defter UPDATE edilmez. Son üretici kimliği4; tek100k henüz başlatılmadı.

Saf altın örnekler/MaxInt64/NULL/eşitlik/negatif-sıfır sınırları PASS (`make test`, /tmp/gp-a8-t3-unit.log). Gerçek API TestEventLimitSnapshotAndPerClaimApproval: iki ayrı CAM onayı her biri1.000.000kuruş; toplam tüketim2.000.000; zeyilde iki sürümde aynı olay limiti; NULL eski sözleşme katalogdan limit devralmaz. TestEventLimitMigrationUpDownUpAndDataGuard PASS. İlk hedefli koşuda genel migration testinin metadata tekrar hazırlığı12kolonunu düşürmeden sürüm kaydını siliyordu; duplicate-column FAIL oldu (/tmp/gp-a8-t3-integration.log). Fixture önce12/11'i Down edip yalnız10'un kayıtlarını silecek şekilde düzeltildi; üretim migrasyonu gevşetilmedi.

`make test-integration`: PASS/exit0, app70,830sn (/tmp/gp-a8-t3-full-integration.log). Simülatör testleri bu pakette çalışmadı. `make lint build`: PASS (/tmp/gp-a8-t3-lint-build.log); YAML ve bash syntax PASS. `T-3 olay-limiti` verify satırı gerçek CAM onayındaki950000kuruş kırpılmayı kontrol eder; tam verify kullanıcı sırasıyla yeni100k sonrasına bekler, henüz bu satıra PASS yazılmadı. ADR-10 güncellendi; UI olay limitini teminat tablosunda ve hesap adımında gösterir.

### 0.B5–11 kapısı

Sistem uçları muhasebe/UW dışına yalnız canlılık sayıları döndürür (simulation=null). Ödeme/iade iş anahtarı poliçe kilidinden sonra kind/refund_of/tutar, zeyil/iptal türüyle doğrulanır; çökme penceresinde yanlış anlam422. Değişmeyen gün sonu audit eklemez. Ekspertiz middleware anahtarı zorunlu, UI/verify/requests/OpenAPI başlığı güncel. Verify plakası3harf4rakam/nanosaniye; BSD date olmayan nanosaniye için mevcut Python3 kullanılır. reset önce compose simulator servisini, sonra varsa adlandırılmış one-off konteyneri durdurur.00010 kesilmiş INVALID indeksi DROP CONCURRENTLY ile kaldırıp yeniden kurar;00001–00009 değişmedi.

`./scripts/test-integration.sh -run 'TestActivity|TestBusinessKeyRejects|TestUnchangedBilling|TestAppraisalIdempotency|TestRefundIndexRecovers|TestBusinessRequestKeyRecovery|TestMigrationsUpDownUp' -v`: PASS/exit0,app13,442sn (/tmp/gp-a8-review-tests.log). Yeni negatif testlerde middleware kaydı silinerek gerçek iş anahtarı kontrolü çalıştırıldı; finansal/audit sayıları değişmedi. Gerçek başarısız UNIQUE CREATE INDEX CONCURRENTLY INVALID indeks bıraktı;00010 doğru FK indeksiyle kurtardı, ödeme sayısı korundu.

`make test-integration`: PASS/exit0,app76,972sn (/tmp/gp-a8-review-full-integration.log); `make test lint build`: PASS/exit0 (/tmp/gp-a8-review-lint-build.log). `make coverage` ilk raporlaması BSD awk ternary ayrıştırmasında FAIL; parantezle düzeltildi, aynı gerçek profilde script PASS: billing92,1/caldate100/pricing95,5/proration97,8/settlement100/fraud100/workflow100/lossratio100%. Hiçbir eşik gevşetilmedi. CI/yük/kaos/EXPLAIN dosyaları hazırlanıyor; bu betiklerin asıl100k çalıştırması henüz yapılmadı.

### Son kodla tek100k başlangıcı

0.B kapısından sonra4155bae kodu ile api/web build ve up--wait PASS.00011/00012 önce mevcut100k üzerinde uygulandı: SIMpoliçe100000, sürüm prim toplamı72130792135kuruş, v0id[5,6,7], v0referansı63742 değişmedi; üç başlangıç2020-01-01, mevcut olay limitleriNULL. Önce/sonra ham kanıt docs/a8-migration-preservation.json. Kullanıcı onaylı reset ardından seed42/anchor2026-10-25/5y/100000/16worker/48h/LIVEfalse tek koşu /tmp/gp-a8-100k.log'a başlatıldı. Oturumlarda synchronous_commit=off korunuyor. Bu koşu sırasında yalnız dosya/belge düzenlemesi; test/yük/kaos/verify yapılmaz.

CI/betik commit0bd3afa; kullanıcı pushladığını bildirdi. GitHub koşusu henüz okunmadı; CIyeşil diye işaretlenmedi. K6resmî2.2.0 imajı digest9bd01d6941fca969cb61bb57d2da5ee9b385fe2aa8881df3798c196564d6ace6; statikinspectPASS. Betik bash/gömülüPython/YAMLsyntaxPASS; asıl ölçümler son100ksonrasına bekliyor. Bütün8domain>=70 kapısı yerel gerçekprofillePASS. READMEiskeleti ölçüm alanlarını BEKLİYOR bırakıyor; Enes'in1/4kişisel yanıtı yer tutucu.

README/TESTING izlenebilirliği Opus tablosuyla karşılaştırıldı; her zorunlu işlev ve teknik teslimin somut testi/dosyası yazıldı. Yeni testlerin tam adları kaynaklarla denetlendi;172ayrı Test* fonksiyonu mevcut (normal ve simülasyon etiketleri ayrı koşular). UzakCI/100k/yük/kaos/EXPLAIN/temizkurulum alanları sonuç gelmeden PASS yapılmadı. ADR-24 kanıt yöntemi, saf domain kapsamının sınırı, kapalıVU yükü ve paylaşılan cache istisnasını kaydeder. EXPLAIN atölye penceresi fraud.Window ile aynı kapalı[loss−30,loss] aralığına düzeltildi; uygulama kuralı değişmedi.

Yük senaryosunun statik kontrolünde demoacente liste kapsamının100k SIMpoliçeyi görmediği fark edildi. scripts/k6/load.js liste için underwriter token'ına bağlandı; ödeme/billing20ayrı demo kabul poliçesinde kalır. Böylece ana tablo100k üzerinde keyset liste ölçülür, yetki/izolasyon kuralı gevşetilmez. Bu dosya değişikliği simülatör üretimini veya uygulama binary'sini etkilemez.

Kabul SQL'i docs/testing/a8-acceptance.sql'a salt okunur hazırlanmıştır. Batch bitince summary ve gerçek sorgu sayımlarını birlikte saklayacak; işaretli kural kırılımı sinyal sayısıdır, aynı hasar birden çok kurala sahip olabileceğinden satırlar birbirini dışlamaz. Açık görev ihlali hem anchor+24h batch kesiminde hem gerçek ölçüm anında kaydedilecek. DONEAPPRAISAL sahipliği NULL'ları da yakalamak için IS DISTINCT FROM ile; eksik ekspertiz kaydı ve CANCELLED karar görevi ayrıca kontrol edilir. Koşu henüz bitmeden bu dosya çalıştırılmadı.

###100k kabul ve ilk ölçümler

Tek v4/100k COMPLETED/DONE,1826,409721sn (CLIreal1829,19sn). Anchor2026-10-25,seed42,16worker,48h; fp32fde797213b8904e5febc4c33cb8bd38f8cd883fdbe1489f02e521226a23b9b.100000poliçe,7780hasar/495işaretli(%6,36),46açıkFRAUD_REVIEW,46açık/9ihlal(%19,57;batchsonu ve gerçekölçümaynı). Pencere dışı açık0, DONEAPPRAISAL sahibi/eksper uyuşmazlığı0, ekspertizsizDONE0,CANCELLEDkarar görevi0. Balanced/receivable_matches true; DB2174540823bayt. Hamkabul/log docs/testing/a8-100k*. İşaretli kombinasyon349EARLY+WORKSHOP,84üçkural,46REPEAT+WORKSHOP,16EARLY+REPEAT; toplam495. Mutlak atölye eşiği sınırlaması sürer.

`/usr/bin/time -p make verify`: PASS62/0,114,26sn; T-3olay-limiti gerçek950000kuruş kırpılma PASS. `/usr/bin/time -p make load`: PASS/exit0,real183,55sn;20VU3dk,24011ölçülenistek, uçların hepsinde%0hata; bütün önceden konan eşiklerPASS. Ham docs/load/before-pool-fix.*; bu ilk imaj4155bae ölçümüdür. MacBookAirAppleM1/8GB/8çekirdek; Docker2CPU/4107407360baytRAM. Kaos/EXPLAIN henüz yapılmadı.

### CI havuz tükenmesi — ek kullanıcı onayı

Kullanıcı4155bae'yi pushladı.40281907975koşusunda backend/frontend/docker/simulationPASS; integrationFAIL(TestConcurrentEndorsements10sn). Transaction bağlantısı tutulurken access.Resolve havuzdan ikinci bağlantı istiyordu;4bağlantılı CI havuzu dolunca aktif iş de kilit bekleyenler de ilerleyemiyor. Kullanıcı minimalDBTX düzeltmesini ve tüm servis taramasını onayladı. Beş bağ ve taranmış güvenli yollar docs/testing/transaction-pool-audit.md'de. Üretim düzeltmesinden önce bariyerli4bağlantı regresyonu gerçekdeadlineFAIL5sn; sonra altısenaryoPASS0,33sn. Mevcut dört paralel mali testle app5,874snPASS. Başta yeni testin assert/require derleme hatası düzeltildi; başarısız sürüm commit'lenmedi.100k tekrar üretilmez; mali kural/API/timeout değişmez. Tam paket ve ikinciCI bekleniyor.

Havuz düzeltmesi sonrası `make test-integration` PASS/exit0,app79,391sn (/tmp/gp-a8-pool-full-integration.log); `make test lint build` PASS (/tmp/gp-a8-pool-lint-build.log). ec8fef8 yerelcommit; kullanıcıdan CI/temizkurulum için push istendi. İlk gerçek k6PASS sonucu before-pool-fix.json/log olarak korundu; yeni imajla aynı eşik/yük tekrar ölçülecek. Yeni migration veya veri reseti yapılmadı.

Temizkurulum disk ön kontrolünde host7,509GiB boş ölçüldü. CURRENT_TASK§7'nin8GB kapısı gereği temizkurulum durdurulacak; kullanıcıdan yaklaşık2GB daha alan açması istendi. Başka dosya/volume silinmedi. Diğer ölçümler devam eder; bu durum temizkurulumPASS değildir.

### Son CI, yük, kaos, EXPLAIN ve temiz kurulum

Yeni40287235419CI(ec8fef8)beşişPASS: backend/docker/integration/frontend/simulation. Kapsam CIgerçekçıktı docs/testing/ci-domain-coverage.md; sonmakecoveragePASS8paket92,1–100%. Sonload20VU3dk24088istek/tümuçlarda%0hata/bütünöncedeneşiklerPASS; CLI182,54sn. Kaosa/b/cPASS12,01sn:503dbdown/JSON5xx,APIaynıid/restart0,iyileşme0,475sn,aynıanahtarteködeme/yevmiye/1200mutabakat.

EXPLAIN3JSONmedyanı:refund178,767→0,171ms;date-report382,542→34,020ms;workshop0,813→0,465ms. İkinciindeksdate-report seçildi; mutlakfark348,522ms vs0,348ms. Hamtxtilkgözlem, tabloJSONmedyanı. Üçindeksrollbacksonrasıvalid/ready. Yeniindeksve00013eklenmedi.

Kullanıcıpushveyeraçmayıbildirdi; başlangıç9580023808bayt boşlukla kapıgeçildi. Aynıpushedec8fef8tazeklon,guvenpolice-a8-clean,yenivolume,özelboşbuilder. Cache0B→1,14GB; cp.env→up--build--wait77,504681sn≤300. Tazeverify62/0,6,22sn. Clone/builderbootstrapölçümdışı; sharedPostgrestabanimajıcache'te; diğerprojelerinpaylaşılanönbelleğisilınmedivekullanılmadı. Hazırlıktabuildxinspect--formatdesteklenmedi; plaindokümana göre düzeltildi. Özelbuilderbootstrapinspectgeçicibağlantıuyarısındansonra buildbaşarılı; gerçek0B/1,14GBcacheçıktılarıkorundu.

Yalnızcloneprojesidown-v;özelbuildervecloneimajaliaslarıkaldırıldı. Ana-volumeName/CreatedAt/Mountpointaynı;100000SIMve3validindekskorundu,üçservishealthy. Anafinalverifyilk61/1FAIL: UIkabulündekapatılanseed2yalnızopenvarsayılmıştı. İhlalfiltrestatus=alloldu; diğer61kontrolzatengeçti. Sonverify62/0,98,65sn (clientbetikfixi e077df7),üretimSLAkodudeğişmedi.

375pxaltırolgerçektarayıcı: müşteri/acenteUNDER_REVIEW/gizlinot/yataytaşma0; hasardaekspertizsizpasifbuton/redipucu;eksper2normalGP-H-2dosyasına#6731ekledi,Doneveeskiatanan4→7audit,sonraADJaktif. Fraudyolundanyenidönüşyok. Muhasebe/UWSistem100k/46/9vemutabakat; müşteriSistemyalnızcanlılık. CAMkırpılma950000kuruşUI'da. Görsellerdocs/testing/ui;sekmetemizlendi/viewportreset. GenelokunabilirlikvebonuslarYAPILMADI;kritikbugvedoğrulamaönceliklendirildi. README1/4kişiselmetinbilerekEnes'ebırakıldı. RepoisPrivate=falseokundu;publicayarı/anketdeğiştirilmedi.

Sonmakecoveragehedefi artık doğrudan daPASS (ilkBSDawkFAILdüzeltmesi doğrulandı). PricinggoldenkomutuPASS0,496sn. A8sondurumJSON08:26UTC: SIM100000,COMPLETED,fpdeğişmedi,3validindeks, SIMeksper/görevuyuşmazlığı0. UIauditgerçekoldassigned4→7. Üçservishealthy. Sonbetik/ölçümparametreleri e077df7'de commit'lendi; uygulama kaynakları CI/temizkurulumun ec8fef8 koduyla aynıdır. .env ve derleme çıktıları stage'e alınmadı.

Kanıt loglarında yalnız satır sonu boşlukları temizlendi; sayılar, hata metinleri ve zamanlar değiştirilmedi. JSON kırılımı okunur biçimlendi. Asıl/tmpkoşu logları korunur. gitdiff--cached--check öncehamDocker/k6satırsonuboşluklarınıyakaladı; temizlemeden commit atılmadı. Ölçüm kanıtları253d679'da.

### A8 dosya manifesti — tam yollar ve amaçlar

| Dosya | Amaç |
|---|---|
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/.env.example](</Users/macair/Okulda yapılan çalışma/guvenpolice/.env.example>) | 48 saatlik açık pencere konfigürasyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/.github/workflows/ci.yml](</Users/macair/Okulda yapılan çalışma/guvenpolice/.github/workflows/ci.yml>) | Beş CI işi, ayrı simülasyon ve domain kapsam kapısı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/CURRENT_TASK.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/CURRENT_TASK.md>) | Onaylı A8/Opus ekleri ve T-3 sırası. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/DECISIONS.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/DECISIONS.md>) | Olay limiti, görev/SLA, mahremiyet ve havuz kararları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/Makefile](</Users/macair/Okulda yapılan çalışma/guvenpolice/Makefile>) | Ayrı simülasyon, kapsam, yük, kaos ve EXPLAIN hedefleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/PROGRESS.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/PROGRESS.md>) | Gerçek koşular, başarısızlıklar, dosya ve commit raporu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/README.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/README.md>) | Kurulum, mimari, ölçümler, sınırlamalar ve kişisel yanıt yer tutucuları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/ROADMAP.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/ROADMAP.md>) | Teknik kapılar tamam; kullanıcı teslim adımları açık. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/TESTING.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/TESTING.md>) | 173 test, kabul yolları, madde–kanıt matrisi ve UI sonuçları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/access/scope.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/access/scope.go>) | Profil sorgusunu mevcut DBTX bağlantısında tut. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/a8_review_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/a8_review_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/a8_tariff_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/a8_tariff_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_adjustment_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_adjustment_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_visibility_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/claim_visibility_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/corrections_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/corrections_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/event_limit_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/event_limit_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/pool_scope_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/pool_scope_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/quote_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/quote_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/router.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/router.go>) | Ekspertiz idempotans middleware bağlantısı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/simulation_fraud_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/simulation_fraud_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/simulation_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/simulation_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/task_completion_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/task_completion_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/verify_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/verify_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/workflow_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/workflow_integration_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/handler.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/handler.go>) | Dış rol mahremiyeti, olay limiti veya transaction bağlantısı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/service.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/service.go>) | Dış rol mahremiyeti, olay limiti veya transaction bağlantısı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/settlement.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/settlement.go>) | Dış rol mahremiyeti, olay limiti veya transaction bağlantısı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/transition.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/transition.go>) | Dış rol mahremiyeti, olay limiti veya transaction bağlantısı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/visibility.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/visibility.go>) | Dış rol mahremiyeti, olay limiti veya transaction bağlantısı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/visibility_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/visibility_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/run.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/run.go>) | İş anahtarı doğruluğu, etkisiz audit ve aynı transaction bağlantısı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/service.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/service.go>) | İş anahtarı doğruluğu, etkisiz audit ve aynı transaction bağlantısı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/replay.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/payment/store/replay.go>) | İş anahtarı doğruluğu, etkisiz audit ve aynı transaction bağlantısı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/change.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/change.go>) | Olay limiti snapshot/zeyil ve güvenli profil sorgusu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/policy.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/policy.go>) | Olay limiti snapshot/zeyil ve güvenli profil sorgusu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/query.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/query.go>) | Olay limiti snapshot/zeyil ve güvenli profil sorgusu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/replay.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/policy/replay.go>) | Olay limiti snapshot/zeyil ve güvenli profil sorgusu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/pricing/catalog.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/pricing/catalog.go>) | Kurgusal CAM olay limiti ve sıralı tazminat hesabı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/quote/service.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/quote/service.go>) | Teklif seçiminde olay limitini snapshot olarak taşı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/fraud.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/fraud.go>) | 2020 v0 ve yeni sözleşme verisini servis kalıbıyla üret. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/policy.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/policy.go>) | 2020 v0 ve yeni sözleşme verisini servis kalıbıyla üret. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/tariffs.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/seed/tariffs.go>) | 2020 v0 ve yeni sözleşme verisini servis kalıbıyla üret. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/settlement/event_limit_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/settlement/event_limit_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/settlement/settlement.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/settlement/settlement.go>) | Kurgusal CAM olay limiti ve sıralı tazminat hesabı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/settlement/settlement_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/settlement/settlement_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/claims.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/claims.go>) | 48 saat pencere, tarihsel kapanış, v4 kimlik ve canlı hata koruması. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/config.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/config.go>) | 48 saat pencere, tarihsel kapanış, v4 kimlik ve canlı hata koruması. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/config_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/config_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/live.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/live.go>) | 48 saat pencere, tarihsel kapanış, v4 kimlik ve canlı hata koruması. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/plan_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/plan_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/policies.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/policies.go>) | 48 saat pencere, tarihsel kapanış, v4 kimlik ve canlı hata koruması. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/run.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/run.go>) | 48 saat pencere, tarihsel kapanış, v4 kimlik ve canlı hata koruması. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/summary.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/simulation/summary.go>) | 48 saat pencere, tarihsel kapanış, v4 kimlik ve canlı hata koruması. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/system/activity.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/system/activity.go>) | Mali simülasyon özetini muhasebe/UW ile sınırla. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/task/repo.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/task/repo.go>) | İşi yapan sahip, DONE red ve tutarlı SLA tanımı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/task/store.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/task/store.go>) | İşi yapan sahip, DONE red ve tutarlı SLA tanımı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/workflow/workflow.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/workflow/workflow.go>) | İşi yapan sahip, DONE red ve tutarlı SLA tanımı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/workflow/workflow_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/workflow/workflow_test.go>) | Kabul kriteri, sınır veya yarış koşulu regresyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00010_payment_refund_index.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00010_payment_refund_index.sql>) | Kesilmiş INVALID indeksi concurrent drop/create ile kurtar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00011_tariff_v0_start.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00011_tariff_v0_start.sql>) | Veriyi koruyarak v0 başlangıcını2020 ile sınırla. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00012_coverage_event_limit.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00012_coverage_event_limit.sql>) | Sürümlü nullable/pozitif olay başı limit ve Down veri koruması. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docker-compose.yml](</Users/macair/Okulda yapılan çalışma/guvenpolice/docker-compose.yml>) | 48 saatlik açık pencere konfigürasyonu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/a8-migration-preservation.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/a8-migration-preservation.json>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/refund-indexed.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/refund-indexed.json>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/refund-indexed.txt](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/refund-indexed.txt>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/refund-without-index.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/refund-without-index.json>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/refund-without-index.txt](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/refund-without-index.txt>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/refund.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/refund.sql>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/report-indexed.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/report-indexed.json>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/report-indexed.txt](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/report-indexed.txt>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/report-without-index.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/report-without-index.json>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/report-without-index.txt](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/report-without-index.txt>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/report.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/report.sql>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/summary.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/summary.json>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/workshop-indexed.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/workshop-indexed.json>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/workshop-indexed.txt](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/workshop-indexed.txt>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/workshop-without-index.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/workshop-without-index.json>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/workshop-without-index.txt](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/workshop-without-index.txt>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/workshop.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/workshop.sql>) | Aynı sorgunun ham indeks karşılaştırması ve medyan kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/load/before-pool-fix.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/load/before-pool-fix.json>) | İlk/son gerçek k6 sonuçları; önceden tanımlı eşikler. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/load/before-pool-fix.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/load/before-pool-fix.log>) | İlk/son gerçek k6 sonuçları; önceden tanımlı eşikler. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/load/final.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/load/final.log>) | İlk/son gerçek k6 sonuçları; önceden tanımlı eşikler. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/load/summary.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/load/summary.json>) | İlk/son gerçek k6 sonuçları; önceden tanımlı eşikler. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-100k-acceptance.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-100k-acceptance.json>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-100k.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-100k.log>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-acceptance.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-acceptance.sql>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-chaos.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-chaos.log>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-ci.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-ci.json>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-final-health.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-final-health.json>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-final-state.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-final-state.json>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-fraud-combinations.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-fraud-combinations.json>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-runbook.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-runbook.md>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-verify-100k.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-verify-100k.log>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-verify-after-ui-failed.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-verify-after-ui-failed.log>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-verify-final.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a8-verify-final.log>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/ci-domain-coverage.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/ci-domain-coverage.md>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/clean-cache-after.txt](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/clean-cache-after.txt>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/clean-cache-before.txt](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/clean-cache-before.txt>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/clean-install-build.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/clean-install-build.log>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/clean-install-cleanup.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/clean-install-cleanup.log>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/clean-install-verify.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/clean-install-verify.log>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/clean-install.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/clean-install.json>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/pool-scope-after.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/pool-scope-after.log>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/pool-scope-before.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/pool-scope-before.log>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/pricing-golden.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/pricing-golden.log>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/transaction-pool-audit.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/transaction-pool-audit.md>) | Gerçek kabul, hata/başarı, kurulum veya CI kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/ui/appraisal-gate-375.jpg](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/ui/appraisal-gate-375.jpg>) | 375 px gerçek tarayıcı kabul görüntüsü. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/ui/appraisal-saved-375.jpg](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/ui/appraisal-saved-375.jpg>) | 375 px gerçek tarayıcı kabul görüntüsü. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/ui/customer-private-375.jpg](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/ui/customer-private-375.jpg>) | 375 px gerçek tarayıcı kabul görüntüsü. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/ui/event-limit-375.jpg](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/ui/event-limit-375.jpg>) | 375 px gerçek tarayıcı kabul görüntüsü. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/ui/system-100k-375.jpg](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/ui/system-100k-375.jpg>) | 375 px gerçek tarayıcı kabul görüntüsü. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/api/types.ts](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/api/types.ts>) | Onaylı hasar görünümü, red ipucu, ekspertiz anahtarı veya olay limiti. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/ClaimDetail.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/ClaimDetail.tsx>) | Onaylı hasar görünümü, red ipucu, ekspertiz anahtarı veya olay limiti. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/ClaimForms.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/ClaimForms.tsx>) | Onaylı hasar görünümü, red ipucu, ekspertiz anahtarı veya olay limiti. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/PolicyDetail.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/components/PolicyDetail.tsx>) | Onaylı hasar görünümü, red ipucu, ekspertiz anahtarı veya olay limiti. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/format.ts](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/format.ts>) | Onaylı hasar görünümü, red ipucu, ekspertiz anahtarı veya olay limiti. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/ClaimsPage.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/ClaimsPage.tsx>) | Onaylı hasar görünümü, red ipucu, ekspertiz anahtarı veya olay limiti. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/SystemPage.tsx](</Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/SystemPage.tsx>) | Onaylı hasar görünümü, red ipucu, ekspertiz anahtarı veya olay limiti. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/openapi.yaml](</Users/macair/Okulda yapılan çalışma/guvenpolice/openapi.yaml>) | Dış hasar durumu, olay limiti, özet yetkisi ve ekspertiz başlığı sözleşmesi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/requests.http](</Users/macair/Okulda yapılan çalışma/guvenpolice/requests.http>) | Ekspertiz için Idempotency-Key örneği. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/chaos.sh](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/chaos.sh>) | Onaylı doğrulama/ölçüm/reset ve test etiketi akışı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/check-coverage.sh](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/check-coverage.sh>) | Onaylı doğrulama/ölçüm/reset ve test etiketi akışı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/explain.sh](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/explain.sh>) | Onaylı doğrulama/ölçüm/reset ve test etiketi akışı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/k6/load.js](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/k6/load.js>) | Onaylı doğrulama/ölçüm/reset ve test etiketi akışı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/load.sh](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/load.sh>) | Onaylı doğrulama/ölçüm/reset ve test etiketi akışı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/reset-demo.sh](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/reset-demo.sh>) | Onaylı doğrulama/ölçüm/reset ve test etiketi akışı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/test-integration.sh](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/test-integration.sh>) | Onaylı doğrulama/ölçüm/reset ve test etiketi akışı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/verify.sh](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/verify.sh>) | Onaylı doğrulama/ölçüm/reset ve test etiketi akışı. |

Toplam 130 dosya; geçici/tmpçalıştırıcıları ve derleme çıktıları bu listeye/commitlere girmez.

### A8 yerel commit listesi (son belge commit'inden önce)

- `5130f39 docs: A8 görev tanımı`
- `a69fb6c fix(tariff): v0 başlangıcını bağlı kayıtları koruyarak 2020 ile sınırla`
- `efb7639 fix(task): kapanan işi yapan kişiye bağla ve karar reddini DONE say`
- `3ad8530 fix(claim): dış rollerde iç inceleme durumunu ve notlarını gizle`
- `db1fa2f fix(verify): mizan ve dönem raporuna 30 saniyelik istemci bütçesi tanı`
- `ab0da04 fix(sim): canlı mod hatasında tamamlanmış veri kümesini koru`
- `dfdafb7 fix(sim): açık işleri 48 saatle sınırla ve kabul testlerini ayır`
- `bccf9bb docs: A8 düzeltme kapısını ve olay limiti onayını kaydet`
- `473cf4e feat(settlement): sürümlü olay başı limit ve gerçek CAM onay kanıtı ekle`
- `670f0b4 fix(review): iş anahtarlarını ve özet yetkisini doğrula, ekspertizi idempotent yap`
- `0bd3afa ci: domain kapsam kapısı, ayrık simülasyon işi ve teslim ölçüm betikleri`
- `4155bae fix(explain): rapor penceresini kayıtlı simülasyon tarihinden al`
- `ec8fef8 fix(db): transaction içindeki yetki sorgularını aynı bağlantıda tut`
- `e077df7 fix(verify): 100k kapsamını ve kapanmış SLA görevlerini doğru doğrula`
- `253d679 test(perf): 100k yük kaos indeks ve temiz kurulum kanıtlarını kaydet`

Son belge commit'inin hash'i final yanıtta ayrıca verilir. Ajan push yapmadı; ec8fef8 Enes tarafından pushlandı, sonraki üç teslim commit'i yereldir.

### A8 teslim özeti — AGENTS §6

1. **Durum: TAMAMLANDI (onaylı teknik kapsam).** Kişisel README1/4 ve sonpush/anket kullanıcıya ait; kesilebilir genel okunabilirlik/bonuslar yapılmadı.
2. **Değişen dosyalar:** Yukarıdaki tam yol/amaç manifesti; backend hesap/iş akışı, üç migration, frontend, CI/betik ve ölçüm kanıtları.
3. **Yapılanlar:** 0.A/0.B1–11, nullable olay limiti/T-3, havuz tükenmesinin tüm servis taraması/düzeltmesi; sonkodla tek100k; CI/yük/kaos/EXPLAIN/temizkurulum ve UI kabulü.00001–00009/audit.go değişmedi; yeni00013yok. 100k46inceleme/46görev/9ihlal, atanan≠eksper0, mali mutabakat true.
4. **Testler:** TESTINGsonkomut tablosu ve kanıt dosyaları. Sonmakecoverage8paket≥70; fullintegration79,391sn; unit/lint/buildPASS; CI40287235419beşişPASS; sonverify62/0,98,65sn; tazeverify62/0,6,22sn; yük24088istek/%0hata/tümeşiklerPASS; kaos12,01sn/0,475sn iyileşme; EXPLAIN17,09sn/rollbackvalid; temizkurulum77,505sn. ÖncekiFAIL'ler gerekçeleriyle korunur.
5. **Eksikler/riskler:** Mutlak atölye eşiği kendi taban hacmine göre anomali değildir. Sınırsız defter raporu ağır olabilir. Ekspertizde middleware iş-commit/yanıt-saklama çökme aralığı kalır (kullanıcı yeterli dedi). SLA oranı ölçüm anına, Sistem özeti batch sonuna aittir. Temizkurulum özelboşbuilder kullanır, paylaşılanPGtabanimajıcache'tedir; clone/bootstrap sayaçdışı. Genelokunabilirlik/bonuslarYAPILMADI. Kişisel yanıt/anket tamamlandı iddiası yok.
6. **Commitler:** Yukarıda15hash+mesaj, finalbelgecommit'i yanıtla. İlk5130f39 docs:A8görevtanımı; üretimfixec8fef8CIyeşil; e077df7betik ve253d679kanıt commit'leri. Push ajan tarafından yapılmadı.
7. **Sonraki görev önerisi:** Enes README1/4kendi metnini eklesin, soncommitleri pushlasın ve publicrepoURL'sini26Ekim23.59TRTöncesi ankete teslim etsin. Yeni aşama/bonus başlatılmadı.

Sonbelgekontrolü: bütün docsJSON'ları parse edildi; README/TESTING/PROGRESS yerelbağlantıları mevcut; OpenAPI/compose/CIYAMLPASS; bash-nscriptsPASS;gitdiff--checkPASS. ec8fef8'den beri backend/frontend/migrationkaynakfarkı0; sonraki farklar kabulbetikleri ve teslim kanıtlarıdır. Soncommitçalışmaağacınıtemizbırakır; kullanıcıteslimadımlarınabaşlanmaz.

## A8.1 kapanışı

Kullanıcının A8.1 görev/onayı önceki salt okunur kapanış görevinin yerine geçti. Başlangıç git durumu temiz/495ce67 idi. Koddan önce docs/testing/a8-acceptance.sql salt okunur oturumda çalıştırıldı (2,449 sn); ham çıktı a81-acceptance-before.json'dadır. Ek kullanıcı onayıyla yalnız measured_at/database_bytes ayrı raporlanır; gerçek saate bağlı SLA ihlali azalırsa veya sabit iş alanları değişirse durulur. Ekspertiz sayısı ve satır özeti ayrıca kaydedildi.

### A — Idempotency middleware envanteri

Router'daki dokuz uç kod değişikliğinden önce incelendi. Normal, saklanmış yanıt tekrarında hepsi middleware replay'i kullanır. Aşağıdaki sonuçlar iş commit'i ile yanıt saklama arasında çökme ve middleware devralması içindir. Ekspertiz dışında ikinci iş/yan etki üreten aynı açık bulunmadı.

| POST ucu | request_key kurtarması | İş kısıtı ve çökme sonrası sonuç |
|---|---|---|
| /api/v1/quotes/{id}/convert | Yok | Teklif FOR UPDATE/durum + policies.quote_id UNIQUE; ikinci poliçe yok, 409 QUOTE_NOT_CONVERTIBLE |
| /api/v1/claims | Var | claims(created_by,request_key) kısmi UNIQUE; poliçe kilidinde aynı hasar/201, yeni fraud/görev yok |
| /api/v1/claims/{id}/appraisals | Önce yok; A8.1 ile var | Önce ikinci ekspertiz/CLAIM_APPRAISED mümkündü. 00013 sonrası appraisals(appraiser_id,request_key) kısmi UNIQUE + hasar kilidinde aynı kayıt/201 |
| /api/v1/claims/{id}/settlement/approve | Yok | Hasar kilidi/durum + claim_settlements_one_active_uq + yevmiye kısıtı; ikinci onay/tüketim yok, 409 INVALID_CLAIM_STATUS |
| /api/v1/claims/{id}/pay | Yok | Hasar kilidi/durum + journal_entries_one_per_event_uq; ikinci ödeme yok, 409 INVALID_CLAIM_STATUS |
| /api/v1/payments/{id}/refund | Var | payments(created_by,request_key) kısmi UNIQUE; kind/refund_of/tutar kontrolü, aynı iade/201 |
| /api/v1/policies/{id}/payments | Var | payments(created_by,request_key) kısmi UNIQUE; kind/tutar kontrolü, aynı tahsilat/201 |
| /api/v1/policies/{id}/endorsements | Var | endorsements(created_by,request_key) kısmi UNIQUE; tür kontrolü, aynı zeyil/201 |
| /api/v1/policies/{id}/cancel | Var | endorsements(created_by,request_key) kısmi UNIQUE; tür kontrolü, aynı iptal/201 |

### B/C — Uygulanan düzeltmeler ve önce/sonra kanıtı

00013 yalnız nullable request_key ve dolu değerler için aktör+anahtar UNIQUE indeksi ekler; mevcut ekspertizlere UPDATE/backfill yok. HTTP middleware'inin context anahtarı kullanılır; simülatör/anahtarsız servis NULL yazar. Aynı hasar kilidi altında mevcut kayıt durum kontrolünden önce bulunur; kurtarma görev/audit eklemez. Farklı anahtarla yeni ekspertiz davranışı korunur. API yanıt alanları değişmedi; anahtar dışarı verilmez.

İlk gerçek regresyon eski kodda FAIL/19,331 sn: hem silinmiş yanıt kaydı hem bayat IN_PROGRESS ikinci ekspertiz ve ikinci audit üretti (a81-appraisal-before.log). İlk düzeltme koşusu FAIL/18,610 sn: ID/sayı/audit doğruydu, test aynı inceleme anının Z/+03:00 gösterimini farklı saydı (a81-appraisal-after.log). Test timestamp alanını PostgreSQL mikrosaniye hassasiyetinde karşılaştırır; diğer alanlar aynı kalmak zorundadır. Nanosecond girişi de kapsanır. Son hedefli koşu PASS/18,981 sn; kurtarma, farklı anahtar, NULL servis yolu, durum ilerledikten sonra kurtarma, en son ekspertizin hesapta korunması, iş anahtarı regresyonları ve migration Up/Down/Up geçti (a81-appraisal-after-canonical-time.log).

Eski ikinci ekspertiz id DESC seçimini değiştirebilirdi: aynı tutarlı kopya tek başına tutarı değiştirmez; araya daha yeni ekspertiz girdiyse eski tutarı yeniden en sona taşıyıp değerlendirme/onay hesabını etkileyebilirdi (claim/settlement.go calculate).

/metrics kullanan yerler tarandı: compose healthcheck, CI, kaos, k6 ve frontend bu ucu çağırmıyor; k6/metrics importu k6'nın yerel ölçüm API'sidir. Verify, üç uygulama entegrasyon testi kullanımı, requests.http ve OpenAPI güncellendi. Formatter'ın bağımsız unit testleri HTTP uygulamasının yetki kuralını test etmediğinden değiştirilmedi. Yeni TestMetricsRequiresFinancialRole: kimliksiz401, CUSTOMER/AGENT/APPRAISER/CLAIMS_ADJUSTER403, ACCOUNTING/UNDERWRITER200; reddedilen gövdelerde ham inceleme etiketi/görev metriği yok. Healthcheck /health üzerinde kaldı.

### D — Sıralı doğrulama

| Adım | Gerçek komut | Sonuç | Toplam süre (sn) |
|---|---|---|---:|
| D1 | `make test lint build` | PASS | 9.239 |
| D2 | `make test-integration` | PASS | 102.531 |
| D3 | `make test-simulation` | PASS | 710.540 |
| D4-services | `docker compose up --build -d --wait --no-deps api web` | PASS | 45.387 |
| D4-verify | `make verify` | 63 PASS / 0 FAIL | 113.737 |
| D5 | `a8-acceptance.sql + saved read-only appraisal query + volume identity comparison` | PASS | 0.736 |

Komut/başlangıç/süre/exit kodları docs/testing/a81-validation.json; ham çıktılar a81-d1/d2/d3/d4*.log içindedir. Ana API olağan açılış migrasyonu 13'ü uyguladı; indeks tanımı ve üç healthy servis a81-main-migration.json'da. DB konteyneri 2a7827096972 olarak korundu; yalnız api/web yeniden oluşturuldu. Verify METRICS-RBAC dahil 63/0.

100k önce/sonra: 100000 poliçe, 7780 hasar, 6727 ekspertiz, 495 işaretli, 46 açık inceleme ve 46 açık görev; balanced/receivable_matches true. Bütün sabit kabul alanları birebir aynı; SLA ihlali9→9, düşüş yok. Kayıtlı parmak izi her iki okumada 32fde797213b8904e5febc4c33cb8bd38f8cd883fdbe1489f02e521226a23b9b. Yeniden hesaplama aracı yok: mevcut fingerprint metodu simülatörün dışarı açılmamış iç metodudur; simülatör çalıştırılmadı/değiştirilmedi. Bu kayıtlı parmak izi okuması yeniden hesaplama olarak sunulmaz.

SIM ekspertizlerinin eski kolonlarından hesaplanan ek MD5 d41e554969cf9f11d9219dde7e732df0 iki okumada aynı; SIM request_key dolu satır sayısı0. guvenpolice_pgdata Name/Driver/Mountpoint/CreatedAt aynı. measured_at 2026-10-26T10:55:29.099134+00:00→2026-10-26T11:27:50.811552+00:00; database_bytes2184485911→2184715287. Bu iki ölçüm alanı ham dosyalarda korunur, eşitlik iddiasına katılmaz. Karşılaştırma a81-comparison.json; ham çıktılar a81-acceptance-before/after.json ve a81-preservation-before/after.json.

100k4155bae'de üretildi. Sonraki bağlantı düzeltmesi yalnız mevcut profil sorgusunun bağlantısını değiştirir; yeni ekspertiz dalı anahtar NULL iken arama yapmaz ve eski alanları aynı değerlerle yazar; anahtarsız yeni kolonu NULL'dır. /metrics yalnız HTTP okuma erişimidir. Simülatör/seed/fraud/billing/task/hesap kaynakları ve00001–00012 değişmedi. Bu nedenle üretilen SIM iş verisi değişmiyor; sayım, ekspertiz satır özeti ve volume kimliği karşılaştırması bunu destekliyor. Ana100k üretimi/reseti veya volume silme/prune yok; simülasyon testlerindeki küçük veri/reset senaryoları geçici test DB'lerindedir. neonsoc_*, macair_esdata ve bağımsız UI önizlemesine müdahale edilmedi.

### E — Belge kapsamı ve sınırlar

ADR-14 request_key kurtarmasını altı uçla sınırlar; diğer üç uçta çökme sonrası409 ile ikinci etki engellenir. ADR-26 metrics yetkisini ve toplu FRAUD_REVIEW sayılarının yan kanal gerekçesini kaydeder. README eksik beş sınırlamayı ekler: kazanılmış prim/UPR/muallak/IBNR, geçmiş faizin geri alınmaması, access token varsayılan15dk iptal edilememesi, giriş hız sınırı olmaması ve kozmetik modül yolu. Mevcut mutlak fraud eşiği ve ağır uç yaklaşık2,7sn maddeleri değişmedi. Eski ekspertiz çift kayıt sınırlaması düzeltildi; kanonik kurtarma yanıtının bayt eşitliği sınırı belirtildi.

README S2 yalnız TESTING.md:411'deki gerçek koşulları kullanır: paralel10k entegrasyon üretimi, mizan10sn/HTTP000, verify279,62sn/58PASS3FAIL. Eşzamanlı HTTP istek sayısı için kayıtlı sayı yok; bu sayıya ilişkin metin eklenmedi. Temiz kurulumun özel0B builder/PGcache koşulu vardı; yalnız ilk PostgreSQL imaj indirmesinin ağ hızına bağlı ve ölçüm dışında olduğu açıklaması eklendi. SLA oranının ölçüm anına bağlı olduğu100k paragrafında zaten yazıyordu; çoğaltılmadı. K6 eşiklerinde yalnız iki noktalı virgül sonrası boşluk düzeltildi. README1/4 metni değişmedi; iki satır SHA256 f6843d0765af77b2b2fdbe6b46f6896602cfed91442cbb9d880de6634f01c0a8 korunur. OpenAPI ve requests.http yeni erişim/kurtarma sözleşmesine bağlandı. Yeni CI/yük/kaos/100k/temizkurulum ölçümü yapılmadı; önceki kayıtlar yeni ölçüm diye sunulmadı.

### Değişen dosyalar

| Tam yol | Amaç |
|---|---|
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/repo.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/repo.go>) | İş anahtarıyla aynı ekspertizi bul. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/service.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/claim/service.go>) | Hasar kilidi altında durumdan önce kurtar; anahtarlı INSERT ve içerik kontrolü. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/idempotency/context.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/idempotency/context.go>) | Yeni UNIQUE çakışmasını mevcut409 desenine bağla. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00013_appraisal_request_key.sql](</Users/macair/Okulda yapılan çalışma/guvenpolice/db/migrations/00013_appraisal_request_key.sql>) | Nullable anahtar ve aktör kapsamlı kısmi UNIQUE; eski veri UPDATE edilmez. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/appraisal_recovery_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/appraisal_recovery_integration_test.go>) | Çökme, farklı/NULL anahtar, son ekspertiz ve migration regresyonları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/integration_test.go>) | Migration13 beklentisi ve yetkili metrics testi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/router.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/router.go>) | Metrics Authenticate + mali rol sınırı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/metrics_access_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/metrics_access_integration_test.go>) | Kimliksiz ve altı rolün metrics erişimi/yanıt gizliliği. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/workflow_integration_test.go](</Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/workflow_integration_test.go>) | SLA ve DB hatası scrape testlerinde muhasebe tokenı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/verify.sh](</Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/verify.sh>) | Yetkili scrape ve tek METRICS-RBAC kabul satırı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/openapi.yaml](</Users/macair/Okulda yapılan çalışma/guvenpolice/openapi.yaml>) | Metrics401/403/rol ve ekspertiz kurtarma sözleşmesi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/requests.http](</Users/macair/Okulda yapılan çalışma/guvenpolice/requests.http>) | Kimliksiz401 örneği, muhasebe scrape ve ekspertiz tekrar açıklaması. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/README.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/README.md>) | Onaylı sınırlamalar, üretim kaynağı, ölçüm koşulları ve S2 gerçek verisi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/DECISIONS.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/DECISIONS.md>) | ADR-14 kapsamı ve ADR-26 erişim gerekçesi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/PROGRESS.md](</Users/macair/Okulda yapılan çalışma/guvenpolice/PROGRESS.md>) | A8.1 envanteri, gerçek sonuçları ve koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-acceptance-after.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-acceptance-after.json>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-acceptance-before.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-acceptance-before.json>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-appraisal-after-canonical-time.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-appraisal-after-canonical-time.log>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-appraisal-after.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-appraisal-after.log>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-appraisal-before.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-appraisal-before.log>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-comparison.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-comparison.json>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-d1-test-lint-build.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-d1-test-lint-build.log>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-d2-integration.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-d2-integration.log>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-d3-simulation-tests.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-d3-simulation-tests.log>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-d4-services.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-d4-services.log>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-d4-verify.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-d4-verify.log>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-main-migration.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-main-migration.json>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-preservation-after.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-preservation-after.json>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-preservation-before.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-preservation-before.json>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-validation.json](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-validation.json>) | A8.1 gerçek önce/sonra, hata/başarı veya süre/koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-doc-check.log](</Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a81-doc-check.log>) | OpenAPI/iç ref, belge koruması ve yerel bağlantı kontrolü. |

Belge kontrolleri PASS: OpenAPI YAML/271 iç ref ve metrics güvenlik sözleşmesi, bash syntax, README1/4 koruması, yedi sınırlama/ölçüm/SLA metni, requests örnekleri,7JSON ve yerel bağlantılar. Sonuç docs/testing/a81-doc-check.log'dadır. Ham test/build logları satır sonu boşlukları dahil değiştirilmeden saklandı; kaynak/belge whitespace kontrolü bu ham çıktıları kapsam dışı bırakır.

### Yerel commitler ve kapanış

- `ce84b1b fix(claim): ekspertiz idempotansını request_key ile iş kaydına bağla`
- `77aa41d fix(api): /metrics erişimini sistem özetiyle aynı rollere sınırla`
- Belge/kanıt commit'inin kendi hash'i final raporda verilir: `docs: A8.1 kapanışı, ADR-14 kapsamı ve sınırlamalar`.

Durum: TAMAMLANDI. D1–D5 PASS; verify63/0. Push yapılmadı. Sonraki öneri yalnız kullanıcının kişisel README1/4 metni ve push/CI/teslim kontrolüdür; bu adımlara başlanmadı.
