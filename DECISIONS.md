# Mimari Karar Kayıtları

A0'da onaylanan beş karar, A2'de (ADR-6/7) ve A3'te (ADR-8/9) eklenenler. Uygulanmayan kısımlar ilgili aşamada kanıtla doldurulacak.

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

**Karar:** `policy_versions` ve `policy_version_coverages` değiştirilemez/silinemez; tek izinli değişiklik açık sürümün `effective_to` alanının bir kez NULL → tarih olarak kapatılmasıdır. `00003_policy_version_guard.sql` ile veritabanında da zorlanır (BEFORE UPDATE/DELETE tetikleyicisi; `to_jsonb(NEW) - 'effective_to' = to_jsonb(OLD) - 'effective_to'` karşılaştırması sonradan eklenecek kolonları da kapsar). Zeyil: açık sürüm zeyil tarihinde kapanır, aynı tarihte yeni sürüm açılır; iptal: açık sürüm iptal tarihinde kapanır, yeni sürüm açılmaz. Hepsi tek transaction, poliçe satırı `FOR UPDATE`.
**Gerekçe:** "Geçmiş bir tarihte geçerli teminat" sorusunun cevabı hiçbir zaman değişmemeli; hasar (A4) bağlandığı sürümün sonradan değişmediğine güvenebilmeli. Uygulama hatası bile geçmişi bozamaz.
**Bedel / sonuçlar:**
- `policy_versions.endorsement_id` kullanılmaz (NULL kalır): şemadaki döngüsel FK'ler ertelenebilir değil ve araç değişikliğinde `endorsements.to_version_id` zorunlu (CHECK); bağı doldurmanın tek yolu sürümü sonradan UPDATE etmek olurdu, bu da kurala aykırı. Sürüm ↔ zeyil bağı `endorsements.from_version_id/to_version_id` ile tutulur.
- **Geriye dönük zincir yok:** zeyil tarihi açık sürümün başlangıcından sonra olmalı; son zeyilden önceki bir tarihe zeyil yapılamaz (422 `INVALID_EFFECTIVE_DATE`).
- **Sıfır uzunlukta sürüm yok:** `CHECK (effective_to > effective_from)` (00001). İptal tarihi açık sürümün başlangıç günüyse (ör. başlangıçtan iptal) sürüm kapatılamaz; açık kalır ve teminat poliçe düzeyindeki `cancel_effective_date` ile sona erer. Bu yüzden "tarihte teminat" sorgusu her zaman poliçe sınırlarını da uygular (`policy.VersionOn`: [start_date, end_date) ve iptal tarihinden önce) — son sürümün `effective_to`'su zaten NULL olduğundan bu kırpma her durumda gerekli.
**Kanıt:** `TestPolicyVersionsAreImmutable` (doğrudan SQL ile UPDATE/DELETE → 55000), `TestMigrationsUpDownUp` (00003 dahil Up/Down/Up), `TestProRataCancellationOnStartDay`.

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
