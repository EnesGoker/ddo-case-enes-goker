# Mimari Karar Kayıtları

A0'da onaylanan beş karar ve A2'de eklenen iki karar. Uygulanmayan kısımlar ilgili aşamada kanıtla doldurulacak.

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
**Durum:** A3'te test edilecek (29 Şubat, yıl sonu).

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
