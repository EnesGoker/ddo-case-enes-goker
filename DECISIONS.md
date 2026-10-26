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

**Karar:** `policy_version_coverages.limit_kurus`, teminatın poliçe dönemi boyunca toplam limitidir; teminat bakiyesi `policy_limit_balances` içindedir. `annual_limit_kurus`, bütün teminatların ortak yıllık toplam limitidir (`coverage_code IS NULL` bakiyesi). A8 onayıyla ayrı olay başı limit eklenmiştir: policy_version_coverages.event_limit_kurus nullable/pozitif kuruştur; NULL sınırsızdır. Bir hasar dosyası bir olaydır; olay limiti her dosyada yeniden uygulanır, dönem boyunca tüketilen ayrı bir sayaç değildir. Hasar tarihindeki sürümün muafiyeti ve limiti kullanılır. Teminat kalan tutarı `max(0, min(sürüm limiti, bakiye limiti) − kullanılmış)`; böylece sonraki limit artışı geçmiş hasarın sözleşme limitini büyütmez. A3'te yıllık limit değişmez.

**Hesap:** Saf `internal/settlement`: son eklenen ekspertizin (`id DESC`) brüt hasarı → muafiyet → muafiyet sonrası → olay başı limit → teminat kalan limiti → yıllık kalan limit → ödeme. Olay limiti hasar tarihindeki sürümden okunur; NULL adımı skipped=true olur. Eski settlement calc_steps ve defter kayıtları yerinde değiştirilmez; yeni hesaplar yedi adım döner. Her adım tutarı, kırpılan miktarı ve nedeni `calc_steps` içinde taşır. Yıllık bakiye yoksa adım `skipped=true`; sabit muafiyet brütle sınırlıdır. Yüzde muafiyeti yarımı yukarıdır; `pricing.MulBP` bölüm/kalan ayrımıyla kullanılır, ara çarpım taşması önlenir. Sıfır ödeme onaylanmaz (`LIMIT_EXHAUSTED`); hasar ADJUSTMENT'ta kalır.

**Kanıt:** `TestGoldenSettlements` (elle hesaplı sabit/yüzde/teminat/yıllık örnekler, muafiyet > brüt ve MaxInt64), `TestInvalidSettlementInput`, `TestZeroSettlementDoesNotConsumeLimit`, `TestGoldenEventLimitsInOrder`, `TestEventLimitInt64AndValidation`, `TestEventLimitSnapshotAndPerClaimApproval`.

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

## ADR-14 — İdempotans çökme penceresi (A6)

`payments`, `endorsements` ve `claims` üzerinde aktör + `request_key` kısmi UNIQUE indeksi vardır (00006). Middleware anahtarı context'e taşır. Servis, poliçe kilidinden hemen sonra, sürüm/durum/bakiye/tarih iş kontrollerinden önce bu anahtarla mevcut kaynağı arar. Otomatik nakit iadede anahtar NULL'dır. Beklenmeyen UNIQUE ihlali 409 IDEMPOTENCY_CONFLICT olur.

Normal tekrarın yanıtı bytea olarak saklanır ve aynı baytlarla döner. Önceden JSONB saklanmış yanıtlar migrasyonda JSONB text biçiminden dönüştürülür; geçmiş ilk HTTP yanıtının özgün baytları geri kazanılamaz. İş commit'i sonrası, yanıt saklanmadan çökülürse bayat IN_PROGRESS devralınır: gövde hash'i önce karşılaştırılır (farklı gövde 422), aynı kaynak ve özgün HTTP durumuyla kanonik güncel yanıt döner; bu kurtarma yanıtı bayt eşitliği vaat etmez. Bildirim kurtarması fraud değerlendirmesini ve görev açmayı tekrar etmez.

| Uç | İş transaction'ındaki koruma |
|---|---|
| Poliçe ödemesi, manuel iade | Poliçe kilidi → payments(created_by,request_key); kind/refund_of ve tutar aynı olmalı, farklıysa422 |
| Zeyil, iptal | Poliçe kilidi → endorsements(created_by,request_key); tür aynı olmalı, farklıysa422 |
| Hasar bildirimi | Poliçe kilidi → claims(created_by,request_key) |
| Teklif çevirme | Teklif kilidi + durum + policies.quote_id UNIQUE |
| Tazminat onayı / ödemesi | Hasar kilidi + durum + claim_settlements.claim_id UNIQUE / durum |
| Ekspertiz ekleme | Idempotency-Key middleware; aynı gövde/anahtar aynı yanıt. Ayrı iş request_key koruması yok; iş commit'i–yanıt saklama çökme penceresi sürer. |

## ADR-15 — Faturalama düzeltmeleri (A6)

Ek prim, `max(etki, bugün)` gününden önce vadeli taksitlere yüklenmez; uygun açık taksit yoksa bu tarihte yeni taksit açılır. Saf billing.Adjust değişmez. Ayardan önce ortak config oranıyla `Accrue(min(etki, bugün))` yapılır; daha önce tahakkuk edilen aralık için yeniden ya da negatif faiz yoktur.

Zeyil/iptal prim ayarı, ayar öncesi faiz yevmiyesi ve otomatik nakit iade (paid_at dahil) işlem gününde kaydedilir. Etki tarihi açıklamada/kayıtta; faiz aralığı from/to'da kalır. Ödeme payment_date, gün sonu as_of, ilk prim tahakkuku poliçe başlangıç günü taşır. Bu ayrım dönem raporunun geriye dönük zeyille kapanmış dönemi değiştirmesini önler.

**Bilinen sınırlama:** Geriye dönük iptalin etki tarihinden sonrası için önceden tahakkuk etmiş faiz iade edilmez. ADR-12'deki kilit kuralı yazma yollarına aittir: GET billing ve değişiklik önizlemeleri A6'da REPEATABLE READ READ ONLY snapshot içinde kilitsizdir. Geçmişinde REJECTED olan CLOSED dosya geriye dönük değişiklik engeline dahil değildir.

## ADR-16 — Fraud skoru, kurgusal danışma sinyali (A6)

Bildirim transaction'ında, hasar tarihine göre bir kez değerlendirilir; yeniden skorlama yoktur. EARLY_CLAIM: başlangıçtan <15 gün, 40 puan; REPEAT_PLATE: normalize plakada bu dosya hariç [hasar−365,hasar] içinde en az 2 dosya, 35 puan; WORKSHOP_CONCENTRATION: aynı atölyede [hasar−30,hasar] içinde en az 4 dosya, 30 puan. Plakasız/atölyesiz girdide ilgili kural uygulanmaz. Eşik 60, koşul skor ≥ eşiktir; tüm değerler env ile ayarlanır ve kurgusaldır.

Sinyal; puan, sayılar, pencere, önceki dosya numaraları ve Türkçe açıklamayla saklanır. İşaretli dosya Transition ile REPORTED → TRIAGED → FRAUD_REVIEW olur. Otomatik geçmiş aktörü NULL, not skoru/eşiği içerir, audit.New.auto=true olur. audit.go ve imzası değişmez; audit çağrısının istek aktörü korunur. Sistem geçişinin NULL aktörü claim_status_history'dedir.

Aynı poliçedeki bildirimler poliçe kilidiyle sıralanır. Farklı poliçelerde eşzamanlı fraud sayımı READ COMMITTED görünümündedir; ±1 kaçırma kabul edilir. Sinyal karar destek içindir, otomatik red üretmez. CUSTOMER/AGENT yanıtları fraud alanlarını içermez.

**A7 ölçüm sınırlaması:** WORKSHOP_CONCENTRATION mutlak dosya adedini ölçer; atölyenin kendi normal hacmine, kapasitesine veya geçmiş ortalamasına göre sapma hesaplamaz. Yoğun çalışan bir atölye normal hacmiyle sinyal üretebilir. A7'de bu kural, puanlar ve eşikler değiştirilmedi; simülatörün atölye çeşitliliği ve kasıtlı örnek yoğunluğu düzeltildi. İşaretli oranı gerçek dolandırıcılık oranı olarak yorumlanamaz.

## ADR-17 — Görev, SLA ve atama (A6)

Saf paket workflow, DB/HTTP paketi task'tır. Transition'a son parametre olarak TransitionOptions{At,SLA,Automatic} eklenir. Eksik zaman veya sıfır/negatif herhangi bir SLA süresi DB erişiminden önce hatadır. APPRAISAL/ADJUSTMENT/FRAUD_REVIEW girişinde görev açılır. Kaynak açık görev DONE, REJECTED'a çıkışta CANCELLED olur; ilk completed_at korunur. İnsan işlemi görevi kapatırken assigned_to işlemi yapana geçer; eski atanan CLAIM_TASK_TAKEN audit kaydında saklanır. Automatic geçişler atamayı değiştirmez. Appraise da açık APPRAISAL görevini DONE yapar. IN_PROGRESS başlatma ucu yoktur.

Atama yalnız aktif kullanıcılardan, tüm tiplerdeki açık OPEN/IN_PROGRESS görev sayısı en az olana, eşitlikte küçük id'ye yapılır. APPRAISAL havuzu APPRAISER, diğer ikisi CLAIMS_ADJUSTER'dır. Sayımdan önce rol başına `pg_advisory_xact_lock(71006,hashtext(rol))` alınır. Boş havuzda assigned_to NULL'dır. Created/due servis saatinden gelir.

SLA takvim saatiyle 72/48/24 saattir (env ile değişir); mesai takvimi yoktur. Açık görevde şimdi > due, DONE görevde completed_at > due ihlaldir; CANCELLED iş yapılmadığı için ihlal/başarı sayılmaz; tam due anı ihlal değildir. Liste done filtresi DONE/CANCELLED içerir; SLA özetindeki done/on_time_bp başarıyla DONE kapanan görevleri ölçer, iptaller bu paydaya girmez. APPRAISER görev listesinde sunucuda zorunlu kendi ataması filtresi vardır.

Metrics Registry scrape sırasında, mutex dışında, tek Summary sorgusuyla ve 250 ms timeout ile iki gauge toplar; başarısız sorguda gauge örnekleri atlanır, eski değer saklanmaz, HTTP 500 oluşmaz.

## ADR-18 — Yazılan prim ve hasar/prim raporu (A6)

Saf oran paketi lossratio, DB/HTTP paketi report'tur. Dönem yevmiye entry_date'ine göre [from,to); eksik sınır açıktır. Yazılan net prim 4000 net alacağıdır (POLICY_ISSUE → poliçe, ENDORSEMENT → zeyil → poliçe); tahakkuk eden tazminat 5000 net borcudur (CLAIM_APPROVAL → hasar → poliçe). Poliçe adedi dönemdeki POLICY_ISSUE kayıtlarıdır. Ay, ürün ve acente aynı hareketlerden gruplanır; acentesiz kayıt ayrı gruptur. Toplam oran, satır oranlarının ortalaması yerine toplam tutarlardan hesaplanır.

Oran int64 baz puandır: tazminat×10000/prim, yarımı sıfırdan uzağa yuvarlanır; prim ≤0 ise null. Çarpım ve bölüm için big.Int kullanılır. Mevcut proration.RoundDiv, MaxInt64 payda sınırında ara toplama taşması ürettiğinden lossratio içinde güvenli bölüm/kalan kullanılır; mevcut A4/A5 hesap kodu değiştirilmez. Sonuç int64'e sığmazsa null döner.

**Sınırlamalar:** Yazılan prim bazlı kurgusal modeldir; kazanılmış prim/UPR, muallak ve IBNR yoktur. POLICY_ISSUE mevcut başlangıç tarihi kuralını korur; zeyil/iptal ve onay işlem günüyle raporlanır.

## ADR-19 — Simülatör ve tarihsel işlem saati (A7)

İş verisi yalnız mevcut policy.Issue, ödeme, iptal ve hasar servisleriyle üretilir. Referans verisi ve simulation_runs doğrudan SQL kullanır. Poliçe/tahsilat evresi varlık başına seed+ordinal RNG ile paraleldir; hasar olayları zaman ve sabit anahtar sırasıyla tek iş parçacığında yürür. Simülatörün ayrı pgx havuzundaki bütün oturumlar synchronous_commit=off kullanır. Çökmede son commit'ler kaybolabilir; yeniden üretim yolu reset-demo ve aynı parametrelerle simulate'tir.

Kasko'nun yaklaşık %12 yıllık toplamı, normal bildirim ve kasıtlı üçlü şüpheli örnekler arasında paylaştırılır; diğer ürünlerin normal sıklığı %6/%4'tür.

**Üretici sürüm 2 (kullanıcı yönlendirmesi):** Atölye havuzu 30'dan 300'e çıkarıldı. Eski 100k koşusunda 7.915 hasarın 1.900'ü (%24,01) işaretliydi; 1.899'unda atölye sinyali, 1.797'sinde kasıtlı üçlü örnek vardı. Bu nedenle yalnız atölye sayısı değil, kasıtlı örnek yoğunluğu da seyreltilir: ilk iki poliçede aynı atölyeye altı yakın dönem bildirimi korunur; sonra her 1.000 poliçede bir yakın dönem ve bir tarihsel üçlü üretilir. Normal Kasko sıklığı %10,7 ile toplam yıllık sıklık yaklaşık %12'de kalır. Parametre sürümü 2 olduğundan eski tamamlanmış koşu yeni üretim sanılarak kullanılmaz; yeniden üretim reset gerektirir.

Seed 42, anchor 2026-10-25, 16 worker ve taze DB ölçümleri: 1k'da 85 hasar / 3 işaretli (%3,53), 10k'da 810 / 44 (%5,43); atölye sinyalleri sırasıyla 2 ve 36, defter dengesi ve 1200 mutabakatı doğru. Bunlar 100k için ölçülmüş oran değildir. Kullanıcı talimatıyla yeni üreticiyle 100k bu gece çalıştırılmadı; sabah A8 ölçümlerinden önce reset-demo → simulate → verify bekliyor. Mutlak eşik sınırlaması ADR-16'da kalır.

Aynı seed, anchor ve iş parametreleriyle taze DB'de aynı iş parmak izi beklenir; worker sayısı iş parametresi değildir. Parmak izi sıralı iş alanlarından üretilir; surrogate id, hasar numarası, sistem zaman damgası ve parola hash'i dışarıda kalır. SIM numaraları sabittir. Tamamlanan aynı koşu yeniden veri üretmez; başka/yarım koşunun mevcut SIM uzayına yazması reddedilir.

Tarihsel v0 tarifeleri 00009'da bağımsız sabitlerle kurulur: 2025 öncesi, v1 faktörleri ve %85 taban fiyatlar (kurgusal). Seed, reset sonrası aynı v0'ı geri kurar; v1/v2 değişmez. İşlem bağlamındaki saat yalnız simülatör ve seed tarafından ayarlanır; ayarlanmayan bağlam gerçek zamanı kullanır. API yolları bağlam saatini ayarlamaz. Servislerin mevcut enjekte saatleri iş kurallarında kalır; alt katman INSERT zamanları bağlam saatini okur. Audit yazma yolu ve audit zaman damgaları değiştirilmez; sonradan toplu tarihselleştirme UPDATE'i yapılmaz.

## ADR-20 — Demo ve simülasyon ayrımı (A7)

SIM- poliçe numaraları, ayrı kurgusal kimlikler, SIM plaka uzayı, acenteler, atölyeler ve rol aktörleri kullanılır. Demo hesapları korunur. Görev atama havuzu ortaktır: assigned_to demo uzmanı olabilir; bu onaylı izolasyon istisnasıdır. Bunun dışındaki müşteri/acente/atölye/oluşturan/ekspertiz aktörleri simülasyona aittir. Reset-demo tüm demo ve simülasyon verisini silerek seed'i yeniden kurar. Canlı mod gerçek saat kullanır ve deterministik değildir; durdurulunca parmak izi null olur, özet batch sonundaki değerdir; LIVE durumunda verify iş mutasyonlarına başlamadan durur.

## ADR-21 — Parçalı gün sonu (A7)

Adaylar aktif ödeme planında as_of öncesi açık vadesi veya açık faizi olan poliçelerdir. Başlangıç şemasındaki installments_open_due_idx ve 00009'daki açık faiz indeksi kullanılır. Poliçe id anahtarına göre varsayılan 500, en çok 2000 kayıt ve gerçek saatle yaklaşık 8 saniyelik başlangıç bütçesi uygulanır. Her poliçe kendi transaction'ında önce poliçe satırını kilitler; ACTIVE olmayan poliçeye iptal uygulanmaz.

İmleç son ziyaret edilendir; kilitli ve başarısız poliçeler de ilerletir. done yalnız aday taraması bitince true olur. skipped bilinen iş istisnaları ve kilitli kayıtlardır; diğer poliçe hataları failed içinde döner ve devam edilir. Context/bağlantı/kaynak/sistem hatası koşuyu durdurur. UI ve verify done olana kadar devam eder; sonraki yeni koşu atlanan/başarısız poliçeleri yeniden deneyebilir.

## ADR-22 — Sistem canlılığı (A7)

Canlılık kaynağı audit_log.occurred_at zamanları ve simulation_runs ilerlemesidir. API tüm giriş yapmış rollere dakikalık sayılar ve son olay zamanını verir; simülasyon ayrıntısı ve mali özet yalnız ACCOUNTING/UNDERWRITER'a döner. Diğer rollerde simulation=null; audit içeriği ve kimlikleri paylaşılmaz. Son 30 dakika sorgusu ve son olay okuması occurred_at indeksiyle sınırlıdır; tablo toplamı alınmaz. Üst çubuk 15 saniyede bir yenilenir ve bağlantı hatasını gösterir.

## ADR-23 — A8 tarih sınırı, dış hasar görünümü ve açık iş penceresi

00011 yalnız v0 başlangıcını2020-01-01'e daraltan onaylı metadata düzeltmesidir; kimlik/fiyat/faktör/poliçe sürümü değişmez.2020 öncesi bağlı poliçe sürümü/teklif varsa transaction hata verir. Seed aynı sınırı kurar; simülatör daha eski başlangıcı DB'ye yazmadan reddeder.00009 değiştirilmez.

CUSTOMER/AGENT bildirim, liste ve detay/geçmiş projeksiyonunda FRAUD_REVIEW yerine UNDER_REVIEW alır. Sistem, incelemeye giriş ve temize çıkış notları gizlenir; red gerekçesi müşteriye gösterilir. İç rollerin görünümü korunur. Demo bildiriminin herkese açık açıklaması nötrdür; gerçek iç bilgiler geçmiş/audit alanlarında kalır.

Simülatör açık iş penceresi SIM_OPEN_WINDOW (varsayılan48h) parametresidir; parametre kimliği sürüm3 olur. Pencere anchor gününün sonundan geriye hesaplanır. Tarihsel yaşam döngüsünün sonu pencere dışına sarkacaksa aynı sıradaki olaylar pencere öncesinde tamamlanacak biçimde sıkıştırılır; eski dosya açık bırakılmaz. Seyrek üçlülerde erken iki olay korunur, üçüncü tekrar/atölye örneği çoğunlukla anchor, bir kısmı önceki güne yerleştirilir; ilk iki örnek aynı gün bildirilerek küçük N'deki patlama korunur. Bu kabul verisi zamanlamasıdır, gerçek hasar dağılımı iddiası değildir. Kurallar ve puan/eşikler değişmez.

A8 olay limiti taşınması: katalogdaki CAM için kurgusal1.000.000kuruş (10.000TL), teminat dönemi limiti2.500.000kuruş. Yeni teklif seçimi, düzenleme ve zeyil sürümüne snapshot olarak kopyalanır. Eski JSON seçimlerde/DB sürümlerinde eksik/NULL değer korunur; güncel katalogdan geriye dönük limit türetilmez.00012 yalnız nullable kolon ekler; sıfır/negatif değer CHECK ile reddedilir. Non-NULL sözleşme varsa Down bilgi kaybını önlemek için başarısız olur.

## ADR-24 — A8 kanıt üretimi ve ölçüm sınırları

CI beş bağımsız iş kullanır: backend build/vet/kapsam, PostgreSQL16 entegrasyon, ayrı ağır simülasyon, Node20 frontend ve imaj build. Simülasyon yalnız main push veya elle tetiklemede; PR temel doğrulamayı ağır100k kabulüne bağlamaz. Kapsam sekiz saf domain paketinde statement ağırlığıyla ayrı ayrı≥%70; düşük bir paketi diğerinin yüksek kapsamı gizleyemez. Servis/HTTP paketlerinin unit kapsamı bu saf domain oranına dahil değildir; gerçek DB/HTTP doğruluğu ayrıca entegrasyon testidir.

K6 ölçümünden önce20VU/3dk ve eşikler betikte sabitlenir. Uç başına özel metrikler setup giriş/fixture yazmalarını dışarıda tutar; yerel JSON yalnız toplu ölçüm içerir. Dış bulut hizmetine sonuç/kimlik bilgisi gönderilmez; resmî imaj digest'i sabittir. Kapalı VU yükü belirli eşzamanlı kullanıcının gecikmesini gösterir; sabit geliş hızı altında azami kapasite iddiası değildir.

İndeks karşılaştırması yalnız simülatör ve yük kapalıyken aynı parametrelerle yapılır. Planner ayarı zorlanmaz. İndekssiz varyant transaction içinde DROP→EXPLAIN→ROLLBACK; hata bağlantıyı kapatır, DDL geri alınır. Ham metin ve üç JSON ölçüm/medyan birlikte saklanır. İlk soğuk/sıcak önbellek ve bu küçük örneğin tüm iş yükünü temsil etmemesi yorumda belirtilir; yalnız daha iyi çıkan süreyi seçip diğer aday gizlenmez.

Temiz kurulum ana100k volume'unu korur. Farklı proje adı/yeni volume ve boş, yalnız bu ölçüme ait buildx builder kullanılır. Paylaşılan varsayılan cache'in hangi projeye ait olduğu güvenle ayrılamadığından küresel prune yapılmaz; ölçüm builder'ı bu cache'i kullanmaz. Paylaşılan PostgreSQL temel imajının önbellekte kalması gibi istisnalar ölçüm kaydında açıkça yazılır. Enes'e ait push/public/anket adımları ajan tarafından yapılmaz.


## ADR-25 — Havuz doluyken transaction içi profil sorgusu (A8)

İkiCPU'lu CI'nin dört bağlantılı pgxpool'unda paralel zeyiller kilitlendi. İşlemler transaction bağlantılarını tutarken access.Resolve pool'dan ikinci bağlantı istedi; aktif işlem boş bağlantı beklerken diğerleri onun poliçe satırını bekledi. Kullanıcı bu üretim hatasının tüm servislerde taranmasını onayladı. Çözüm kapasite/timeout artırmak değil, mevcut db.DBTX ile yetki sorgusunu aynı transaction'a bağlamaktır. Policy target/replay, payment target ve claim visible bu bağı taşır; transaction dışındaki çağrılar pool kullanabilir. Beş bağ ve diğer servislerin neden güvenli olduğu docs/testing/transaction-pool-audit.md'dedir. Dört bağlantıyı aynı anda tutan bariyerli regresyon eski kodda deadlineFAIL, yeni kodda altı senaryodaPASS verdi. API/RBAC/para/kilit sırası değişmez; yeni migration ve100k tekrarına gerek yok.

Sonv4/100k işaretli oranı495/7780=%6,36; atölye sinyali479işaretli dosyada, kombinasyonlar349EARLY+WORKSHOP/84üçlü/46REPEAT+WORKSHOP/16EARLY+REPEAT.300atölye kabul hedefini karşılar; eşik hâlâ atölyenin kendi taban hacmine göre değil mutlak önceki dosya adedidir. İstatistik doğrulaması kural/eşik değiştirmek için kullanılmadı.
