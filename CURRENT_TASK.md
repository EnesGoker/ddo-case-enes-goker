# CURRENT_TASK — Sadece aktif ve onaylı iş

**Aktif aşama:** A2 — T-1 + F-1: tarifeli prim motoru, teklif, teklifi poliçeye çevirme

**Durum:** ONAYLI (A2'ye başla). A3'e geçmek için yeni onay gerekir.

**Süre kutusu:** ~4 saat üst sınır. Takılırsan dur ve raporla.

## A1 sonrası kararlar

- A1 raporu kabul edildi. `legacy-peer-deps` ve `typescript 5.9.3` sabitlemesi kalsın; gerekçesini `DECISIONS.md`'ye bir satır olarak ekle.
- `CURRENT_TASK.md` her aşama başında commit'lenebilir (`docs: A2 görev tanımı`).
- Temiz önbellekte 5 dakika ölçümü A8'de yapılacak; şimdi değil.

## 1. Önce: üç ortak yardımcı (A1'den devreden)

1. **`Idempotency-Key` middleware'i** (`idempotency_keys` tablosu):
   - Anahtar zorunlu olan uçlarda yoksa 400 `IDEMPOTENCY_KEY_REQUIRED`.
   - `request_hash` = method + path + gövde hash'i. Aynı anahtar + aynı hash → kayıtlı yanıt aynen döner (durum kodu + gövde). Aynı anahtar + farklı hash → 422 `IDEMPOTENCY_KEY_REUSED`.
   - `IN_PROGRESS` iken gelen ikinci istek → 409 `IDEMPOTENCY_IN_PROGRESS`. `IN_PROGRESS` kaydı belirli süreden (env, varsayılan 60 sn) eskiyse yeniden işlenebilir.
   - Yalnızca 2xx ve 4xx iş hataları kaydedilir; 5xx kaydedilmez (tekrar denenebilsin).
   - Testler: tekrar istek aynı yanıt, farklı gövde 422, **eşzamanlı aynı anahtarla 5 istek → tek iş etkisi**.
2. **Audit yardımcısı:** `audit_log`'a aynı transaction içinde yazar (aktör, rol, işlem, varlık, eski/yeni JSON, request-id).
3. **Keyset sayfalama yardımcısı:** `?limit=&cursor=`; cursor opak (base64), `limit` üst sınırı 100. `customers` listesi bu yardımcıya geçirilsin.

## 2. T-1 prim motoru (asıl iş)

**Tasarım kuralları:**
- Motor **saf bir Go paketi** (`internal/pricing`): girdi = risk verisi + teminat seçimi + tarife sürümü (yüklenmiş struct); çıktı = satır satır döküm + toplam. Veritabanı, saat, rastgelelik yok. Böylece deterministiklik ve test kolay.
- **Tarife seçimi:** teklif dönemi başlangıç tarihine (`term_start`) göre, `valid_from <= term_start < valid_to` olan `ACTIVE` sürüm. İstek anındaki saat değil.
- **Hesap sırası (sabit, dokümante edilsin):** her teminat için taban prim → AGE → NCD (hasarsızlık) → REGION → VEHICLE_GROUP çarpanları sırayla. Her adımdaki fark (kuruş) bir döküm satırı. KONUT için araç faktörleri yok; ürün başına geçerli faktör listesi tarifeden gelir.
- **Yuvarlama:** her çarpan adımında kuruşa, yarımı yukarı (half-up), tam sayı aritmetiği (`bp` ile). Float yok. Toplam = döküm satırlarının toplamı, **her zaman tam eşit** (fark varsa `ROUNDING` satırı yazılır; normalde oluşmamalı).
- Vergi/harç **yok** (varsayım olarak dokümante et); gerek görürsen tek bir `TAX` satırı ekleyebilirsin ama zorunlu değil.
- `input_hash` = normalize edilmiş girdi + tarife sürüm id'sinin sha256'sı. Aynı girdi + aynı tarife → aynı hash ve aynı sonuç.
- **Underwriter kuralı (basit, tek kural yeter):** örn. sürücü yaşı < 21 veya toplam prim belirli eşiğin üstündeyse teklif `UW_REVIEW` olur; değilse `CALCULATED`. Eşik tarifede ya da env'de, dokümante edilsin.

**Seed tarifeleri (kurgusal, açıkça örnek diye işaretli):**
- Üç ürün için `ACTIVE` sürüm. **KASKO için iki sürüm:** v1 (geçmiş tarihli, `valid_to` dolu) ve v2 (şu an geçerli, farklı oranlar) — böylece tarih bazlı seçim kanıtlanır.
- Yaş için `num_range` aralıkları, NCD için en az 4 kademe, 3-4 bölge, 3-4 araç grubu, kısa dönem tablosu (A3 kullanacak ama şimdi seed'lensin).

## 3. Uç noktalar (F-1 + çevirme)

- `GET /api/v1/tariffs/active?product=&date=` — AGENT, UNDERWRITER.
- `POST /api/v1/quotes/preview` — AGENT. Hesaplar, **kaydetmez** (form canlı hesap için).
- `POST /api/v1/quotes` — AGENT. Hesaplar ve kaydeder, dökümle döner.
- `GET /api/v1/quotes` (keyset), `GET /api/v1/quotes/{id}` (döküm dahil) — AGENT kendi, UNDERWRITER tümü, CUSTOMER kendi.
- `POST /api/v1/quotes/{id}/review` — UNDERWRITER. `APPROVED` / `REJECTED` + not. Audit yazılır.
- `POST /api/v1/quotes/{id}/convert` — AGENT, `Idempotency-Key` zorunlu. Yalnızca `CALCULATED` veya `APPROVED` teklif. Tek transaction'da:
  - `policies` (benzersiz `policy_no`), `policy_versions` v1 (`effective_from = start_date`, `effective_to = NULL`), `policy_version_coverages`,
  - `policy_limit_balances` (her teminat + yıllık toplam limit için, poliçe dönemi),
  - teklif → `CONVERTED`, audit kaydı.
  - Taksit planı ve muhasebe kaydı **A5'te**, şimdi yok.
  - Aynı teklif iki kez çevrilemez (`quotes.status` kilidi + `policies.quote_id` UNIQUE).
- Sahiplik kontrolü sunucuda: acente başka acentenin teklifini göremez/çeviremez (403 veya 404, tutarlı olsun).

## 4. Frontend (en küçük hali)

- Acente: teklif formu (ürün, müşteri, risk alanları, teminat seçimi) → preview dökümü tablo olarak (her faktör kaç TL) → kaydet → poliçeye çevir. Teklif listesi.
- Underwriter: `UW_REVIEW` teklif listesi + onayla/reddet.
- Müşteri: kendi teklif/poliçe listesi (sadece liste).
- Yükleniyor/boş/hata durumları, 375 px'de kullanılabilir. Cila yok.

## 5. Kabul kriterleri ve testler

**Birim (saf motor, `internal/pricing`):**
- Elle hesaplanmış en az **2 altın örnek** (KASKO ve KONUT): her döküm satırı ve toplam kuruşu kuruşuna tutuyor. Hesabı test dosyasında yorum olarak adım adım yaz.
- Determinizm: aynı girdi 100 kez → aynı sonuç ve hash.
- Döküm toplamı = nihai prim (tablo testi, çok sayıda girdi).
- Sınırlar: yaş aralığı sınırları (`[)`), NCD kademeleri, yarım kuruş yuvarlama.
- Tarife seçimi: `valid_to` günü yeni sürüm, bir gün önce eski sürüm seçilir.
- Domain paketinde kapsam ≥ %70 (`go test -cover` çıktısını raporla).

**Entegrasyon (gerçek PostgreSQL):**
- Teklif kaydet → oku → döküm aynı. Aynı girdiyle ikinci teklif aynı toplam ve hash.
- Geçmiş tarihli teklif v1, güncel teklif v2 tarifesini kullanıyor.
- Convert: poliçe + sürüm + teminatlar + limit bakiyeleri oluşuyor; aynı `Idempotency-Key` ile tekrar → aynı yanıt, tek poliçe; **farklı anahtarlarla eşzamanlı iki convert → tek poliçe**, diğeri 409/422.
- RBAC: UNDERWRITER teklif oluşturamaz (403), CUSTOMER başkasının teklifini göremez, AGENT başka acentenin teklifini çeviremez.

**`make verify`'a eklenecek kimlikler:** `F-1` ve `T-1` satırları (gerçek API çağrısıyla: iki kez hesapla → eşit; döküm toplamı = toplam; tarih bazlı sürüm seçimi; convert idempotansı). Her biri ayrı PASS/FAIL.

**Komutlar:** `go test ./...`, `./scripts/test-integration.sh`, `npm run build && npm run lint`, `make verify`.

## Dokümanlar

- `openapi.yaml` yeni uçlarla güncellensin.
- `DECISIONS.md`: yuvarlama ve hesap sırası kararı, tarife seçim kuralı.
- `TESTING.md`: F-1 ve T-1 için "nasıl denenir / beklenen sonuç" + kanıt komutu.
- `requests.http`: yeni uçlar.
- README'deki doğrulama sorusu 5 (prim motoru kanıtı) için kullanılacak komut ve gerçek çıktıyı `PROGRESS.md`'ye not et.

## Kapsam dışı

Zeyil/iptal/pro-rata (A3), hasar (A4), taksit/muhasebe (A5), simülatör, CI, `git push`.
