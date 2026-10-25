# CURRENT_TASK — Sadece aktif ve onaylı iş

**Aktif aşama:** A3 — T-2 + F-2: poliçe sürümleri, zeyil, iptal, gün esaslı iade

**Durum:** ONAYLI (A3'e başla). A4'e geçmek için yeni onay gerekir.

**Süre kutusu:** ~4 saat üst sınır. Takılırsan dur ve raporla.

## 0. A2'den küçük düzeltmeler (önce bunlar)

1. **UNIQUE ihlali 500 olmasın:** PostgreSQL `23505` (unique_violation) iş kısıtlarında domain hatasına çevrilsin (ör. `policies.quote_id` → 409 `QUOTE_ALREADY_CONVERTED`). Kilit asıl koruma, bu ikinci savunma hattı. Mutasyon denemesini tekrarla: kilit yokken artık 409 dönmeli.
2. İdempotent tekrar yanıtında anahtar sırasının değişebilmesi kabul; `PROGRESS.md`'ye bilinen sınırlama olarak yaz, değiştirme.

## 1. Sürüm kuralları (karar)

- **Sürümler değişmez (immutable).** Bir sürümde değişebilen tek alan, açık sürümün `effective_to` alanının **bir kez** `NULL` → tarih olarak kapatılmasıdır. Diğer tüm alanlar ve kapanmış sürümler değiştirilemez.
- Bunu veritabanında da zorla: **`db/migrations/00003_policy_version_guard.sql`** ile `policy_versions` ve `policy_version_coverages` için `BEFORE UPDATE/DELETE` tetikleyici (yalnızca `effective_to IS NULL → NOT NULL` geçişine izin). Up/Down/Up dene.
- Zeyil: açık sürümü `effective_to = zeyil tarihi` ile kapat, `effective_from = zeyil tarihi` olan yeni sürüm aç. Hepsi tek transaction, poliçe satırı `FOR UPDATE` ile kilitli.
- **Geriye dönük zincir yok:** zeyil tarihi, açık sürümün `effective_from`'undan büyük ve poliçe `end_date`'inden küçük olmalı; değilse 422. (Son zeyilden önceki bir tarihe zeyil yapılamaz; bilinen sınırlama olarak dokümante et.)
- İptal edilmiş poliçeye zeyil yapılamaz (422).

## 2. Hesap kuralları (karar, `DECISIONS.md`'ye yaz)

Tüm hesap **saf bir pakette** (`internal/endorsement` ya da `internal/proration`), DB'siz, tam sayı aritmetiği, yarımı yukarı yuvarlama.

- **Dönem gün sayısı** = `end_date - start_date` (gerçek gün; içinde 29 Şubat varsa 366). Kalan gün = `end_date - zeyil/iptal tarihi`. Geçen gün = `tarih - start_date`. Aralıklar `[)`.
- **Araç değişikliği zeyli:**
  - Yeni yıllık prim, poliçenin **orijinal tarife sürümüyle** (`policy_versions.tariff_version_id`) ve yeni risk verisiyle T-1 motorundan hesaplanır. (Zeyil tarihindeki tarife değil; gerekçe: sözleşme şartları düzenleme anında sabitlenir.)
  - `premium_delta = round_half_up((yeni_yıllık - eski_yıllık) × kalan_gün / dönem_günü)`; pozitif = ek prim, negatif = iade.
  - Teminat limitleri isteğe bağlı değişebilir; yeni limit, o teminatta kullanılmış tutarın altına düşerse 422 (`LIMIT_BELOW_USAGE`). Limit bakiyeleri buna göre güncellenir (`limit_kurus` değişir, `used_kurus` korunur).
- **İptal:**
  - Müşteri talebi (`CUSTOMER_REQUEST`): **kısa dönem tarifesi**. Kazanılmış prim = `yıllık_prim × kısa_dönem_oranı(geçen_gün)`. İade = toplam yazılmış prim − kazanılmış prim (negatifse 0).
  - Şirket kaynaklı (`UNDERWRITER`, `NON_PAYMENT`): **gün esaslı (pro-rata)**. İade = `yıllık_prim × kalan_gün / dönem_günü`.
  - "Toplam yazılmış prim" = ilk prim + tüm zeyil deltaları. Tahsilat ile mahsup **A5'te**; şimdi iade tutarı yalnızca hesaplanıp `endorsements.calc_detail` ve `premium_delta`'ya yazılır.
  - İptal: açık sürüm `effective_to = iptal tarihi` ile kapanır, yeni sürüm açılmaz; `policies.status = CANCELLED`, `cancel_effective_date`, `cancel_reason` dolar.
  - Bu kısa dönem / gün esaslı ayrımı kurgusal iş varsayımıdır; öyle dokümante et.
- `calc_detail` JSON'u adım adım yazılsın: dönem günü, geçen/kalan gün, artık yıl var mı, kullanılan oran, eski/yeni yıllık prim, yuvarlama öncesi/sonrası.

## 3. Uç noktalar

- `GET /api/v1/policies/{id}` — poliçe + tüm sürümler + teminatlar + zeyiller (A2'de eksikti). Erişim: A2'deki kapsam kuralı.
- `GET /api/v1/policies/{id}/coverage?date=YYYY-MM-DD` — o tarihte geçerli sürüm ve teminatlar. Poliçe dışı tarih veya iptal sonrası → 404 `NO_COVERAGE_ON_DATE` (tutarlı bir kod seç).
- `POST /api/v1/policies/{id}/endorsements/preview` — kaydetmeden hesap dökümü (AGENT, UNDERWRITER).
- `POST /api/v1/policies/{id}/endorsements` — araç değişikliği zeyli. AGENT, UNDERWRITER. `Idempotency-Key` zorunlu. Audit.
- `POST /api/v1/policies/{id}/cancel/preview` ve `POST /api/v1/policies/{id}/cancel` — iptal (gerekçe zorunlu). AGENT (yalnızca `CUSTOMER_REQUEST`), UNDERWRITER (her gerekçe). `NON_PAYMENT` A5'teki otomatik kural için ayrılır; elle de verilebilir ama yalnızca UNDERWRITER. `Idempotency-Key` zorunlu. Audit.

## 4. Testler ve kabul kriterleri

**Saf hesap paketi (birim, kapsam ≥ %70):**
- Artık yıl: 2027-06-01 → 2028-06-01 dönemi 366 gün; 2026-01-01 → 2027-01-01 dönemi 365 gün. 2028-02-29 tarihli zeyil ve iptal.
- Sınırlar: başlangıç gününde iptal (geçen 0 gün), son günde iptal (kalan 1 gün), kısa dönem tablo sınırları (`[)`).
- En az **2 altın örnek** (bir araç değişikliği, bir kısa dönem iptal) elle hesaplanmış, adımlar test dosyasında yorum olarak.
- Pro-rata delta işareti (prim artışı/azalışı) ve yuvarlama.

**Entegrasyon (gerçek PostgreSQL):**
- Zeyil sonrası `coverage?date=` sorgusu: zeyil tarihinde yeni sürüm, bir gün önce eski sürüm. İptal tarihinde ve sonrasında 404.
- Sürüm değişmezliği: kapanmış sürüme doğrudan `UPDATE` denemesi DB tarafından reddediliyor.
- **Eşzamanlılık:** aynı poliçeye farklı anahtarlarla eşzamanlı iki zeyil → biri başarılı, diğeri tutarlı bir hata (409/422); sürüm aralıkları çakışmıyor, `endorsement_no` tekil. Eşzamanlı zeyil + iptal → tutarlı son durum.
- İdempotans: aynı anahtarla tekrar zeyil → aynı yanıt, tek zeyil.
- Limit kullanılanın altına düşürülürse 422 (kullanımı test içinde doğrudan bakiyeye yazarak simüle et; hasar A4'te).
- RBAC: AGENT `UNDERWRITER` gerekçesiyle iptal edemez (403); CUSTOMER zeyil/iptal yapamaz.

**`make verify`:** `F-2` ve `T-2` satırları gerçek API ile: poliçe üret → zeyil → tarih bazlı teminat sorgusu (önce/sonra) → kısa dönem iptal ve beklenen iade tutarı → iptal sonrası teminat yok.

## 5. Frontend (en küçük hali)

- Poliçe detayı: sürüm zaman çizelgesi (tablo yeter), seçilen tarihte geçerli teminat sorgusu.
- Acente/UW: araç değişikliği formu → önizleme dökümü (gün sayıları, delta) → onayla. İptal → önizleme (iade tutarı) → onayla.
- Müşteri: poliçe detayını salt okunur görür.
- Yükleniyor/boş/hata ve 375 px.

## Dokümanlar

`openapi.yaml`, `DECISIONS.md` (sürüm değişmezliği, pro-rata/kısa dönem kuralı, zeyilde orijinal tarife), `TESTING.md` (F-2, T-2), `requests.http`, `ROADMAP.md` kanıt matrisi.

## Kapsam dışı

Tahsilat/muhasebe kaydı ve taksit planı düzeltmesi (A5), hasar (A4), `EXPIRED` işi, `git push`.
