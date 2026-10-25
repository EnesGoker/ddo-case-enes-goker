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

**Durum: TAMAMLANDI.** A1–A3 kalıpları ve çalışan testler korundu; 00001/00002/00003 değiştirilmedi. İlk commit yalnızca onaylı CURRENT_TASK.md oldu (`4749fbd`, `docs: A4 görev tanımı`). A5'e başlanmadı, push yapılmadı.

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

- `4749fbd docs: A4 görev tanımı`
- `fc9853e fix(policy): zeyil bağlantısını bir kez doldur ve hasarlı tarihlere değişikliği engelle`
- `7abcdbb feat(settlement): adımlı tam sayı tazminat hesabı ve dengeli yevmiye yardımcısı`
- `7890706 feat(claim): hasar, ekspertiz, limit kilitli onay ve idempotent ödeme`
- `5e77fa8 feat(web): altı rol için hasar dosyası, ekspertiz ve tazminat akışı`
- `c7db57e test(claim): yarış testini kilitte buluştur ve giriş/geri alma kontrollerini güçlendir`
- `0840f50 fix(web): hasar işlemlerinden sonra dosya listesini yenile`
- `f8754ca feat(verify): F-4 F-5 F-6 ve paralel yıllık limit kanıtları`
- `64e2fc6 style(claim): yeni paketlerde mevcut import gruplarını uygula`

Son doküman/biçim commitleri `git log 15fe058..HEAD` ve teslim raporunda yer alır; push yok.

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

- `fd659dc docs: A5 görev tanımı`
- `ae55cf3 feat(billing): saf taksit, faiz, tahsis ve prim ayarı hesapları`
- `fb0817b feat(policy): düzenleme ve zeyil iptal akışını ödeme planı ve deftere bağla`
- `e513dff feat(payment): tahsilat, iade, gün sonu ve defter mutabakatı`
- `07e7bf1 feat(web): taksit seçimi, ödeme iade ve mizan ekranları`
- `eb41b35 test(verify): F-3 tahsilat ve T-4 paralel ödeme faiz mutabakat kanıtları`

- `3fb6bea test(payment): manuel iadede faiz tahsisinin yeniden açılmasını doğrula`

Son A5 doküman commitinin hash'i `git log 204fd8d..HEAD` ve teslim raporunda bulunur; push yapılmadı.

### Sonraki görev

Yalnızca öneri: A6 — fraud, atama/SLA ve raporlar. Yeni onay olmadan başlanmaz.
