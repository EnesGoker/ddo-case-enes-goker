# Mimari Karar Kayıtları

A0'da onaylanan beş karar; A2 (ADR-6/7), A3 (ADR-8/9) ve A4 (ADR-10/11) ekleri. Uygulanmayan kısımlar ilgili aşamada kanıtla doldurulacak.

## ADR-1 — Modüler monolit, katmanlı paketler

**Karar:** Tek Go binary'si; her iş alanı `internal/<alan>` paketinde handler → service → repo (+ saf domain) katmanlarıyla. Tek React uygulaması, rol bazlı ekranlar.
**Gerekçe:** Teslim süresi kısa; tek kişi/tek ajan; transaction sınırları (limit, tahsilat, defter) aynı veritabanında. Mikroservis dağıtık transaction ve operasyon yükü getirirdi.
**Elenen:** Mikroservis + kuyruk (gereksiz karmaşıklık), ORM (sorgu ve kilit davranışının açık görünmesini istiyoruz).
**Durum:** A1'de uygulandı (`auth`, `customer`, `health`, `platform/*`).

## ADR-2 — Para ve oranlar tam sayı

**Karar:** Para `int64` kuruş (DB `BIGINT`), oranlar `bp` (1/10.000) ve `ppm` (1/1.000.000) tam sayı. Yuvarlama tek noktada, satır bazında, yarımı yukarı; döküm satırları toplamı nihai prime eşit.
**Gerekçe:** Float ikili gösterimi kuruş hatası üretir; tam sayı deterministik ve test edilebilir.
**Durum:** A2'de uygulandı (ADR-6).

## ADR-3 — Tarih aralıkları `[başlangıç, bitiş)` ve gerçek gün sayısı

**Karar:** Tüm aralıklar yarı açık. Pro-rata, poliçe döneminin gerçek gün sayısıyla (365/366) hesaplanır; kısa dönem oranı tarife tablosundan.
**Gerekçe:** Sınır günlerinin iki dilime birden düşmesini engeller; PostgreSQL `daterange '[)'` ve EXCLUDE kısıtlarıyla uyumlu.
**Durum:** A3'te uygulandı ve test edildi (ADR-9; `internal/proration`: 2027-06-01→2028-06-01 = 366 gün, 2028-02-29 tarihli zeyil/iptal, yıl sonu).

## ADR-4 — Eşzamanlılık ve idempotans

**Karar:** Limit tüketimi koşullu `UPDATE ... WHERE used + x <= limit`; taksit/tahsilatta `SELECT ... FOR UPDATE`; kritik POST'larda `Idempotency-Key` (`idempotency_keys`, `IN_PROGRESS` → `COMPLETED`, tekrar istekte kayıtlı yanıt).
**Gerekçe:** Uygulama içi mutex çok replikada çalışmaz; veritabanı tek doğruluk kaynağı.
**Durum:** A1'de refresh rotasyonunda, A2'de teklif çevirmede (`SELECT ... FOR UPDATE` + `Idempotency-Key`) uygulandı; eşzamanlı 5 istekle test edildi. Limit/tahsilat A4–A5'te.

## ADR-5 — Kimlik doğrulama: kısa ömürlü JWT + dönen refresh cookie

**Karar:** 15 dk HS256 access token (yalnızca bellekte, `Authorization: Bearer`). Refresh token 256 bit rastgele, DB'de yalnızca SHA-256 özeti; httpOnly + SameSite=Strict cookie, `Path=/api/v1/auth`. Her yenilemede döner; kullanılmış token tekrar gelirse kullanıcının tüm refresh token'ları iptal edilir. Parolalar bcrypt (cost 12). nginx `/api`'yi aynı origin'den API'ye iletir.
**Gerekçe:** XSS ile refresh token çalınamaz; CSRF'e karşı SameSite=Strict + access token'ın header'da olması; stateless erişim denetimi + iptal edilebilir oturum.
**Bedel:** Paralel iki yenileme isteği oturumu düşürür; frontend yenilemeyi tek uçuşa indirir (`refreshSession`). Access token iptal edilemez (15 dk pencere).
**Durum:** A1'de uygulandı ve test edildi.

## ADR-6 — Prim hesap sırası ve yuvarlama (A2)

**Karar:** Saf `internal/pricing` paketi (DB/saat/rastgelelik yok). Her teminat için sabit sıra: taban prim → AGE → NCD → REGION → VEHICLE_GROUP. Tarifede tanımlı olmayan faktör tipi o üründe uygulanmaz (KONUT: yalnızca NCD + REGION). Her çarpan adımında `yeni = (önceki × bp + 5000) / 10000` (kuruşa yarımı yukarı, tam sayı); adımın farkı bir döküm satırıdır. Toplam = satırlar toplamı; fark çıkarsa açık `ROUNDING` satırı yazılır (testlerde hiç oluşmadı). Vergi/harç yok (varsayım).
**Gerekçe:** Adım adım yuvarlama her satırı "bu faktör kaç TL" olarak açıklanabilir kılar ve toplamın satırlardan sapmasını yapısal olarak imkânsız hale getirir. Tek seferde yuvarlamada satırlar kesirli kalır, açıklama zorlaşır.
**Bedel:** Faktör sırası değişirse sonuç birkaç kuruş değişebilir; bu yüzden sıra sabit ve dokümante.
**Determinizm:** `input_hash` = fiyatı etkileyen normalize girdi (yalnızca uygulanan faktörler + katalog sırasındaki teminatlar) + tarife sürüm id'sinin SHA-256'sı. Aynı hash → aynı sonuç (100 tekrar ve faktör sırası karıştırılarak test edildi).
**Kanıt:** `TestGoldenKasko`, `TestGoldenKonut` (elle hesap test dosyasında adım adım), `TestBreakdownSumEqualsTotal` (5.000+ kombinasyon).

## ADR-7 — Tarife seçimi poliçe başlangıç tarihine göre (A2)

**Karar:** Teklif için tarife, istek anına göre değil `term_start` gününe göre seçilir: `status = 'ACTIVE' AND valid_from <= term_start < valid_to` (valid_to NULL = süresiz). Seçim tek bir saf fonksiyonda (`pricing.SelectTariff`); veritabanındaki EXCLUDE kısıtı aynı üründe çakışan iki ACTIVE sürümü zaten engeller.
**Gerekçe:** Prim, sigortanın başladığı günün tarifesiyle hesaplanmalı; istek saati teklifin tekrar hesaplanmasında sonucu değiştirmemeli (determinizm).
**Kanıt:** `TestSelectTariffByStartDate` (valid_to günü yeni sürüm, bir gün önce eski), entegrasyonda `TestQuoteUsesTariffValidOnTermStart`, `make verify` `T-1 tarife-tarihi`.

## ADR-8 — Poliçe sürümleri değişmez (A3)

**Karar:** `policy_versions` ve `policy_version_coverages` değiştirilemez/silinemez; izinli değişiklikler açık sürümün `effective_to` alanının bir kez NULL → tarih olarak kapatılması ve A4 `00004` ile `endorsement_id` alanının bir kez NULL → değer olarak doldurulmasıdır. `00003_policy_version_guard.sql` ile veritabanında da zorlanır (BEFORE UPDATE/DELETE tetikleyicisi; `to_jsonb(NEW) - 'effective_to' = to_jsonb(OLD) - 'effective_to'` karşılaştırması sonradan eklenecek kolonları da kapsar). Zeyil: açık sürüm zeyil tarihinde kapanır, aynı tarihte yeni sürüm açılır; iptal: açık sürüm iptal tarihinde kapanır, yeni sürüm açılmaz. Hepsi tek transaction, poliçe satırı `FOR UPDATE`.
**Gerekçe:** "Geçmiş bir tarihte geçerli teminat" sorusunun cevabı hiçbir zaman değişmemeli; hasar (A4) bağlandığı sürümün sonradan değişmediğine güvenebilmeli. Uygulama hatası bile geçmişi bozamaz.
**Bedel / sonuçlar:**
- A4 `00004_policy_endorsement_link.sql`: döngüsel FK nedeniyle yeni sürüm önce NULL bağlantıyla eklenir; zeyil eklendikten sonra aynı transaction içinde `endorsement_id` bir kez doldurulur. İkinci yazma/NULL yapma ve başka alanlarla birlikte değiştirme reddedilir. A3 kayıtları yerinde değiştirilmez.
- **Geriye dönük zincir yok:** zeyil tarihi açık sürümün başlangıcından sonra olmalı; son zeyilden önceki bir tarihe zeyil yapılamaz (422 `INVALID_EFFECTIVE_DATE`).
- **Sıfır uzunlukta sürüm yok:** `CHECK (effective_to > effective_from)` (00001). İptal tarihi açık sürümün başlangıç günüyse (ör. başlangıçtan iptal) sürüm kapatılamaz; açık kalır ve teminat poliçe düzeyindeki `cancel_effective_date` ile sona erer. Bu yüzden "tarihte teminat" sorgusu her zaman poliçe sınırlarını da uygular (`policy.VersionOn`: [start_date, end_date) ve iptal tarihinden önce) — son sürümün `effective_to`'su zaten NULL olduğundan bu kırpma her durumda gerekli.
**Kanıt:** `TestEndorsementLinkIsWrittenOnce`, `TestPolicyVersionsAreImmutable` (doğrudan SQL ile UPDATE/DELETE → 55000), `TestMigrationsUpDownUp` (00003 dahil Up/Down/Up), `TestProRataCancellationOnStartDay`.

## ADR-9 — Zeyil ve iptalde prim hesabı (A3)

**Karar:** Saf `internal/proration` paketi (DB/saat yok, tam sayı). Dönem gün sayısı = `end − start` (gerçek gün; 29 Şubat varsa 366), geçen = `tarih − start`, kalan = `end − tarih`, aralıklar `[)`.
- **Araç değişikliği:** yeni yıllık prim, poliçenin **orijinal tarife sürümüyle** (`policy_versions.tariff_version_id`) ve yalnızca araç alanları değişmiş risk girdisiyle T-1 motorundan hesaplanır (sözleşme şartları düzenleme anında sabitlenir). `delta = (yeni − eski) × kalan / dönem`.
- **İptal (KURGUSAL iş varsayımı):** müşteri talebi → kısa dönem (kazanılmış = açık sürüm yıllık primi × oran(geçen gün), iade = yazılmış − kazanılmış, ≥ 0); şirket kaynaklı (UNDERWRITER, NON_PAYMENT, OTHER) → gün esaslı (iade = yıllık × kalan / dönem, yazılmış primle sınırlı). Yazılmış prim = ilk prim + tüm zeyil farkları. Tahsilatla mahsup A5'te; iade şimdilik yalnızca `endorsements.premium_delta_kurus` (negatif) ve `calc_detail`'e yazılır.
- **Yuvarlama:** kuruşa yarımı yukarı; negatif tutarlarda mutlak değer üzerinden (sıfırdan uzağa, `HALF_AWAY_FROM_ZERO`). Böylece +X ve −X yıllık fark aynı büyüklükte delta üretir (−2,5 → −3).
- `calc_detail` adım adım: dönem günü, artık yıl, geçen/kalan gün, kullanılan oran/bant, eski/yeni yıllık prim, pay/payda, yuvarlama öncesi (4 ondalık) ve sonrası, okunur Türkçe adımlar.
**Gerekçe:** Hesap ile veri erişimini ayırmak artık yıl/sınır/yuvarlama durumlarını DB'siz ve deterministik test etmeyi sağlar; önizleme ve kayıt aynı plan fonksiyonundan geçer (önizlenen = kaydedilen).
**Kanıt:** `TestGoldenVehicleChange` (199.670 kuruş), `TestGoldenShortPeriodCancellation` (250.150 kuruş), `TestProRataCancellation` (721.924 kuruş), `TestCancellationOnLeapDay`, `TestCancellationBoundaries`; `make verify` T-2 satırları (beklenen tutarlar betikte bağımsız yeniden hesaplanır).

## Ek notlar (A3)

- **İyimser sürüm kontrolü:** zeyil/iptal isteğinde isteğe bağlı `base_version_no`; önizlemeden sonra poliçe değiştiyse 409 `STALE_VERSION` (önizlenen tutar ile kaydedilenin farklı çıkmasını önler). Arayüz her zaman gönderir.
- **İkinci savunma hattı:** kilit atlanırsa sürüm/zeyil benzersizlik ve dışlama kısıtları 409 `CONCURRENT_MODIFICATION`'a, `policies.quote_id` 409 `QUOTE_ALREADY_CONVERTED`'a çevrilir (`db.Violates`); açık sürümü kapatma ve poliçeyi iptal etme koşullu UPDATE'tir (`WHERE effective_to IS NULL`, `WHERE status = 'ACTIVE'`).
- **Limit değişikliği** zeyilde katalog limitine kadar düşürülebilir/geri yükseltilebilir; prim etkilenmez (örnek tarifede prim teminat başına sabit tabandan hesaplanır). Kullanılan tutarın altına düşemez (422 `LIMIT_BELOW_USAGE`; bakiyede koşullu UPDATE `used_kurus <= yeni limit`).
- **Rol kuralı:** zeyil AGENT/UNDERWRITER; iptalde AGENT yalnızca `CUSTOMER_REQUEST` (diğer gerekçeler 403), `NON_PAYMENT` elle yalnızca UNDERWRITER (otomatik kural A5'te).

## Ek notlar (A2)

- **Yaş:** müşterinin doğum tarihinden `term_start` gününe göre tamamlanmış yaş; 29 Şubat doğumlular artık olmayan yılda yaşını 1 Mart'ta tamamlar (varsayım). Bölge müşteri kaydından gelir.
- **Dönem:** `term_end = term_start + 1 yıl`; 29 Şubat + 1 yıl = 1 Mart (Go normalizasyonu, varsayım; A3'te gün hesabıyla birlikte test edilecek).
- **Teminat limitleri/muafiyetleri** şemada tarife tablosu olmadığı için kodda kurgusal bir katalogdadır (`pricing.Catalog`); teklif oluşturulurken `coverage_selection`'a kopyalanır, poliçe bu kopyadan düzenlenir — katalog sonradan değişse de eski teklif/poliçe etkilenmez.
- **Underwriter kuralı (kurgusal):** sürücü < 21 yaş veya toplam prim > `UW_PREMIUM_THRESHOLD_KURUS` (varsayılan 3.000.000 kuruş = 30.000 TL) → `UW_REVIEW`. Nedenler `risk_data.review_reasons` içinde saklanır.
- **Erişim:** rol uca hiç erişemiyorsa 403; erişebiliyor ama kayıt kapsamı dışındaysa 404 (başkasına ait kaydın varlığı sızdırılmaz).
- **Idempotency:** `IN_PROGRESS` → handler → 2xx/4xx `COMPLETED` (5xx'te kayıt silinir). İş transaction'ı ile anahtarın COMPLETED yazılması ayrı adımdır; aradaki çökme durumunda kayıt `IDEMPOTENCY_STALE_AFTER` (60 sn) sonra devralınır ve iş kuralı (teklif zaten CONVERTED → 409) ikinci etkiyi engeller. JSONB biçimi değiştirdiği için tekrar yanıtı sıkıştırılarak döner (içerik aynı, anahtar sırası farklı olabilir).
- **Poliçe no:** `GP-<KS|TR|KN>-<başlangıç yılı>-<teklif no 8 hane>`; teklif–poliçe 1:1 (UNIQUE) olduğu için benzersiz, ek sequence gerekmez.
- **Geriye dönük poliçe yok:** teklif geçmiş başlangıçla hesaplanabilir (tarife geçmişi gösterimi), ama `term_start < bugün` ise çevirme 422.
- **Frontend bağımlılıkları:** npm 10.8.2 peer çözümleme hatası (`edgesOut`) nedeniyle `frontend/.npmrc` içinde `legacy-peer-deps=true`; `typescript-eslint` peer aralığı (<6.1) nedeniyle TypeScript 5.9.3'e sabit.

## Ek notlar (A1)

- Migrasyonlar API açılışında goose ile uygulanır; demo seed idempotenttir ve `DEMO_PASSWORD` değişirse hash'i günceller.
- `/metrics` ek bağımlılık yerine elle yazılmış küçük bir Prometheus metin üreticisidir; etiket olarak ham yol yerine chi rota kalıbı kullanılır (kardinalite).

## ADR-10 — Tazminat limiti modeli ve tam sayı hesabı (A4)

**Karar:** `policy_version_coverages.limit_kurus`, teminatın poliçe dönemi boyunca toplam limitidir; teminat bakiyesi `policy_limit_balances` içindedir. `annual_limit_kurus`, bütün teminatların ortak yıllık toplam limitidir (`coverage_code IS NULL` bakiyesi). Ayrı olay limiti yoktur; bu bir **kurgusal iş varsayımıdır**. Hasar tarihindeki sürümün muafiyeti ve limiti kullanılır. Teminat kalan tutarı `max(0, min(sürüm limiti, bakiye limiti) − kullanılmış)`; böylece sonraki limit artışı geçmiş hasarın sözleşme limitini büyütmez. A3'te yıllık limit değişmez.

**Hesap:** Saf `internal/settlement`: son eklenen ekspertizin (`id DESC`) brüt hasarı → muafiyet → muafiyet sonrası → teminat kalan limiti → yıllık kalan limit → ödeme. Her adım tutarı, kırpılan miktarı ve nedeni `calc_steps` içinde taşır. Yıllık bakiye yoksa adım `skipped=true`; sabit muafiyet brütle sınırlıdır. Yüzde muafiyeti yarımı yukarıdır; `pricing.MulBP` bölüm/kalan ayrımıyla kullanılır, ara çarpım taşması önlenir. Sıfır ödeme onaylanmaz (`LIMIT_EXHAUSTED`); hasar ADJUSTMENT'ta kalır.

**Kanıt:** `TestGoldenSettlements` (elle hesaplı sabit/yüzde/teminat/yıllık örnekler, muafiyet > brüt ve MaxInt64), `TestInvalidSettlementInput`, `TestZeroSettlementDoesNotConsumeLimit`.

## ADR-11 — Hasar iş akışı, kilit sırası ve mali kayıt (A4)

**Karar:** Bütün durum değişiklikleri tek `claim.Transition` üzerinden; claim `FOR UPDATE`, `row_version` kontrolü, `claim_transitions` doğrulaması, durum/geçmiş/audit aynı transaction'da. İlk bildirim `NULL → REPORTED` geçmişiyle birlikte yazılır. Bildirimde poliçe satırı zeyil/iptalle aynı kilidi alır; `policy.VersionOn` hem sürümü hem poliçe/iptal sınırlarını uygular. Reddedilmemiş bir hasar tarihinde veya öncesinde zeyil/iptal `CLAIMS_AFTER_EFFECTIVE_DATE` ile reddedilir (önizleme dahil).

**Onay kilit sırası:** claim → ilgili teminat/yıllık bakiye satırları **id ASC**. Kilit altında hesap; her bakiyede `used + ödeme <= limit` koşullu UPDATE ikinci savunmadır. Settlement, tüketimler, APPROVED geçişi, audit ve `CLAIM_APPROVAL` yevmiyesi tek transaction: 5000 borç / 2000 alacak. Ödeme claim kilidi altında: PAID geçişi, settlement `paid_at` / `payment_journal_entry_id`, audit ve `CLAIM_PAYMENT`: 2000 borç / 1000 alacak. `ledger.Post` satırsız, negatif/çift yönlü ve dengesiz kayıtları reddeder; başarısızlık bütün transaction'ı geri alır. Her iki mali uç ve bildirim mevcut idempotans middleware'ini kullanır; ADR-4/A2'deki iş transaction'ı–yanıt saklama aralığı sınırlaması sürer.

**Sınırlama:** APPROVED geri alınamaz; uzlaşma iptali bu aşamada yoktur, `claim_settlements.CANCELLED` kullanılmaz. Eksper görünürlüğü APPRAISAL dosyaları veya kendi ekspertizi bulunan dosyalardır; görev ataması/fraud/SLA A6'ya aittir.

**Kanıt:** `TestConcurrentClaimApprovals` iki bağımsız HTTP isteğini dış transaction kilidinde buluşturur; 10 tekrar ve `-race` geçti. Bakiye `FOR UPDATE` geçici kaldırılınca ikinci onay `409 LIMIT_CHANGED` aldı; koşullu UPDATE taşmayı engelledi, tam/kırpılmış iki başarı bekleyen test FAIL oldu. Kilit geri konuldu ve 10 tekrar `-race` PASS. `TestClaimLifecycleAndIdempotency` tek settlement/yevmiye, ödeme ve defter dengesini; `TestClaimValidationAndLedgerRollback` eksik muhasebe hesabında geri almayı doğrular.

## ADR-12 — Taksit, tek poliçe kilidi ve tahsis (A5)

**Karar:** Saf `internal/billing` hesaplar; `internal/payment` uygulama/HTTP, `internal/payment/store` çağıranın transaction'ında mali kayıtlar. `policy` yalnızca `payment/store` kullanır; store policy paketini import etmez. Düzenleme/zeyil/iptal/ödeme/iade/tahakkuk ve plan okuma önce aynı poliçe satırını `policy.LoadHeader(..., true)` ile kilitler. Gün sonu önce `FOR UPDATE SKIP LOCKED`, sonra aynı başlık yardımcısı; kilitli poliçe `skipped/LOCKED` döner. Zeyil/iptal mevcut transaction ve iş kurallarını kullanır; A4 koduna genel refactor yapılmaz.

- 1–12 taksit, toplam kuruştan fazla taksit yok; bölüm ve kalan ilk taksite. Vade, orijinal başlangıca ay eklenerek hedef ay sonuna kırpılır (`caldate.AddMonths`); önceki kırpılmış vade zincirlenmez.
- Tahsis `(due_date, seq)` sırası; her taksitte eski faiz önce, sonra anapara. Erken ödeme serbest, fazla ödeme bütünüyle 422 `OVERPAYMENT`. Tahsis tutarı daima pozitif, yön `payments.kind`'dan gelir.
- Tek `billing.Adjust`: ek prim uygun açık taksitlere (vade >= etki tarihi) dağıtılır; yoksa yeni seq ile ek taksit. Negatif prim en geç açık vadeden azaltılır. Ödenmemiş sıfır bakiye taksiti tutarı korunarak CANCELLED; kısmen ödenmişte yeni amount=paid, PAID. Artan prim azalışı en yeni tahsilatların iade edilebilir kalanından tahsissiz REFUND olur.
- `cash_refund_kurus` müşteriye dönen nakit; A3'ün prim azalışı/iade hesabından ayrıdır. Plan toplamı iptal olmayan taksitlerin toplamıdır; peşin ödenmiş tutarlar burada kalabilir. 00005 yalnızca plan toplamı CHECK'ini `>=0` yapar; Down `>0` kuralına döner, sıfır plan varsa veri değiştirmeden başarısız olabilir.
- Manuel iade yalnızca ACTIVE poliçede; özgün ödemenin tahsisleri en sondan geriye alınır (önce anapara, gerekirse faiz). Bugüne kadar mevcut borç faizlenir; iade ile açılan anapara iade tarihinden sonraki tahakkukta işler. Otomatik prim iadesi tahsis geri almaz.

**Kanıt:** Altın hesaplar, gerçek PostgreSQL entegrasyonları, her mali senaryonun sonunda ortak defter/plan/mutabakat yardımcısı. İki ödeme dış transaction ile taksit UPDATE'inde buluşturulur: normal 201 + 422; poliçe kilidi mutasyonu 201 + 409 `CONCURRENT_PAYMENT`, CHECK taşmayı engeller. Kilit geri konularak 10 tekrar `-race` PASS.

## ADR-13 — Faiz, 30 gün ve çift taraflı prim muhasebesi (A5)

**Karar (kurgusal/sadeleştirilmiş):** Prim düzenlemede gelir yazılır; kazanılmamış prim karşılığı **yoktur**. Faiz basit, tam sayı kuruş, yarımı yukarı: ödenmemiş anapara × günlük ppm × gün / 1.000.000. Ara çarpım `math/big`, sonuç int64 sınırında doğrulanır. `LATE_INTEREST_DAILY_PPM` varsayılan 1000 (%0,1/gün); bileşik faiz yok. Aralık `[max(vade, son tahakkuk to, anaparayı yeniden açan iade tarihi), as_of)`; iptal edilmiş poliçede bitiş iptal etki tarihinden ileri gidemez. Vade gününde 0 gün faiz.

| Olay | Borç / alacak | ref_type / ref_id |
|---|---|---|
| Düzenleme | 1200 / 4000 | POLICY_ISSUE / poliçe |
| Ek prim | 1200 / 4000 | ENDORSEMENT / zeyil |
| Prim azalışı/iptal | 4000 / 1200 | ENDORSEMENT / zeyil |
| Faiz | 1200 / 4100 | LATE_INTEREST / faiz kaydı |
| Tahsilat | 1000 / 1200 | PAYMENT / ödeme |
| Manuel/otomatik nakit iade | 1200 / 1000 | REFUND / iade ödemesi |

A4'ün 2000/5000 kayıtları aynen korunur. `ledger.Post` her olayda satırlı ve dengeli kayıt üretir. Değişmez: `1200 (borç−alacak) = Σ(PENDING/PARTIAL amount−paid) + Σ(faiz amount−paid)`. Mizan endpoint'i repeatable-read anlık görüntüde genel denge, her yevmiyenin dengesi/satırları ve mutabakatı okur.

**Gün sonu:** Açık taksitin gecikmesi >=30 günse ACTIVE poliçeyi A3 `CancelTx` ile `NON_PAYMENT/as_of` iptal eder; 29 gün yeterli değildir. Aktör muhasebe kullanıcısıdır; elle iptalin rol/gerekçe kuralı router'da korunur. A4 `CLAIMS_AFTER_EFFECTIVE_DATE` koruması veya geçersiz etki tarihi kayıt bazında raporlanır; diğer hatada ilgili poliçenin transaction'ı geri alınır. Her poliçe ayrı commit olduğundan sonraki bir hata önceki başarıları geri almaz. Aynı tarih yeni faiz/iptal üretmez (audit yazılabilir). Otomatik zamanlayıcı yok; endpoint dışarıdan çağrılır.

**Sınırlar:** A5 öncesi plansız poliçe için otomatik backfill yok, 409 `BILLING_PLAN_MISSING`; yalnızca silinebilir demo ortamında kullanıcı `make reset-demo` kullanabilir. Bu aşamada mevcut veri silinmedi. İdempotans mevcut middleware ile normal tekrar/eşzamanlı istekte tek mali etki sağlar; iş transaction'ının commit'i ile yanıtın saklanması ayrı olduğundan süreç çökmesindeki tekrar işleme aralığı sürer (A2/A3 sınırlaması). Banka entegrasyonu, gerçek sigorta tarifesi/mevzuatı, kazanılmamış prim, bileşik faiz ve iptal sonrası manuel iade kapsam dışıdır.
