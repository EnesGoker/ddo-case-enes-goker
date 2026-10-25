# CURRENT_TASK — Sadece aktif ve onaylı iş

**Aktif aşama:** A5 — T-4 + F-3: taksit planı, tahsilat, gecikme faizi, iade, 30 gün kuralı, çift taraflı muhasebe

**Durum:** ONAYLI. A6'ya geçmek için yeni onay gerekir.

**Ajan notu:** A4'ü yapan ajan devam ediyor. Kurallar aynı: `AGENTS.md`, `PROGRESS.md`, `DECISIONS.md` oku; mevcut kalıpları (`policy.Issue`, `policy.LoadHeader(…, true)`, `ChangeService.Cancel`/zeyil akışı, `ledger.Post`, `idempotency`, `audit`, `access`, `page`, `caldate`, `proration.RoundDiv`, test yardımcıları ve A4'teki "gate" yarış testi tekniği) **yeniden kullan**, kopyalama ve refactor yok. `PROGRESS.md`'ye "A5 (ajan: Codex)" bölümü yaz. İlk commit: `docs: A5 görev tanımı`.

**Süre kutusu:** ~4 saat üst sınır.

## 0. Temel kararlar (bunları `DECISIONS.md`'ye yaz)

- **Tek kilit:** tüm faturalama işlemleri (plan, ödeme, iade, faiz, gün sonu, zeyil/iptal ayarı) önce poliçe satırını `FOR UPDATE` ile kilitler (`policy.LoadHeader(…, true)`). A3 zeyil/iptal zaten bu kilidi alıyor; A5 ayarı aynı transaction'da çalışır.
- **Muhasebe modeli (kurgusal, sadeleştirilmiş):** yazılan prim düzenleme anında gelir yazılır (kazanılmamış prim karşılığı yok — bilinen sınırlama). Hesaplar: `1000 Banka`, `1200 Prim Alacakları`, `4000 Prim Gelirleri`, `4100 Gecikme Faizi Gelirleri` (A4'ün `2000`/`5000` kayıtları aynen kalır).

  | Olay | Yevmiye | `ref_type` / `ref_id` |
  |---|---|---|
  | Poliçe düzenleme | B 1200 / A 4000 (toplam prim) | `POLICY_ISSUE` / poliçe id |
  | Zeyil ek prim (+) | B 1200 / A 4000 | `ENDORSEMENT` / zeyil id |
  | Zeyil iadesi veya iptal (−) | B 4000 / A 1200 | `ENDORSEMENT` / zeyil id |
  | Gecikme faizi tahakkuku | B 1200 / A 4100 | `LATE_INTEREST` / faiz kaydı id |
  | Tahsilat | B 1000 / A 1200 | `PAYMENT` / ödeme id |
  | Ödeme iadesi (manuel veya fazla ödeme iadesi) | B 1200 / A 1000 | `REFUND` / iade ödeme id |

- **Mutabakat değişmezi (T-4'ün "bakiye doğrulaması"):** her an `1200 bakiyesi (borç − alacak) = Σ(açık taksitlerin amount − paid) + Σ(faiz kayıtlarının amount − paid)`. Açık = `PENDING`/`PARTIAL`; `PAID` katkısı 0; `CANCELLED` taksitler hariç (yalnızca `paid = 0` iken iptal edilir). Ayrıca tüm defterde toplam borç = toplam alacak ve her yevmiye kendi içinde dengede.
- Para tam sayı kuruş, yuvarlama yarımı yukarı (`proration.RoundDiv` / mevcut yardımcı). Float yok.

## 1. Saf hesap paketi (`internal/billing`, DB'siz, kapsam ≥ %70)

1. **Taksit bölme:** `n` taksit (1–12, `n ≤ toplam`), eşit böl, kalan kuruşlar **ilk** taksite. Vade: `i`. taksit = `start_date + (i−1) ay`, ay sonu kırpmalı (31 Ocak → 28/29 Şubat, 31 Mart → 30 Nisan). `caldate`'e `AddMonths` ekle + testleri.
2. **Gecikme faizi:** basit faiz, bileşik değil. `faiz = round_half_up(ödenmemiş_anapara × günlük_ppm × gün / 1_000_000)`. Tahakkuk aralığı `[from, to)`: `from = max(vade, son tahakkuk to'su)`, `to = as_of` (iptal edilmiş poliçede `min(as_of, cancel_effective_date)`). Vade gününde ödeme = 0 gün faiz. Oran env `LATE_INTEREST_DAILY_PPM`, varsayılan `1000` (%0,1/gün, kurgusal).
3. **Ödeme tahsisi:** taksitler `(due_date, seq)` sırasıyla; her taksitte **önce o taksitin ödenmemiş faizi** (eski faiz önce), sonra anaparası. Toplam açık bakiyeyi aşan ödeme → hata (`OVERPAYMENT`, tamamı reddedilir). Erken ödeme serbest (en eski açık taksite gider).
4. **Prim ayarı (tek fonksiyon; zeyil +/− ve iptal hepsi bunu kullanır):**
   - `delta > 0`: vadesi `≥ etki tarihi` olan açık taksitlere eşit dağıt, kalan ilke; pay 0 olan taksite dokunma. Hiç yoksa vadesi etki tarihi olan yeni taksit ekle (`seq = max+1`; `installment_count` ilk sayıyı temsil eder, değişmez).
   - `delta < 0` (R = |delta|): açık taksitleri **en geç vadeliden başlayarak** azalt: `alınan = min(amount − paid, kalan R)`. Sonuç `amount == paid` olursa: `paid == 0` → `CANCELLED` (amount değişmez), `paid > 0` → `PAID`. Açık bakiye yetmezse **artan tutar = müşteriye nakit iade**: ödemelere en yeniden başlayarak `REFUND` kayıtları (`refund_of_payment_id`, her biri o ödemenin iade edilebilir kalanını aşmaz), tahsis yok, yevmiye `REFUND`.
   - `installment_plans.total_kurus` = iptal olmayan taksitlerin toplamı, her ayarda güncel tutulur.
5. **30 gün kuralı:** açık bir taksitte `as_of − due_date ≥ 30` gün → poliçe iptal adayı. Sınır testi: 29 gün aday değil, 30 gün aday.

Her biri için elle hesaplanmış altın örnekler (yorumda adım adım): bölme+kalan, ay sonu vadeleri, faiz yuvarlama, tahsis (faiz→anapara, birden çok taksite yayılan kısmi ödeme), negatif ayar + nakit iade, pozitif ayarın dağıtımı.

## 2. Akışlara bağlama

- **Düzenleme:** `POST /quotes/{id}/convert` isteğe bağlı `installment_count` alır (varsayılan 1). `policy.Issue` ile aynı transaction'da plan + taksitler + `POLICY_ISSUE` yevmiyesi. Frontend'deki çevirme ekranına taksit sayısı seçimi.
- **Zeyil/iptal (A3):** `premium_delta` kaydedildiği transaction'da prim ayarı (Bölüm 1.4) + yevmiye. Önizleme uçları da müşteriye nakit iade tutarını göstersin (`cash_refund_kurus`); A3'teki "prim iadesi" ile karıştırılmasın (prim azalışı ≠ nakit iade). Plansız (A5 öncesi) poliçede ayar → 409 `BILLING_PLAN_MISSING` (reset-demo ile çözülür; dokümante et).
- **Seed (demo için, aynı servis kodu ile, doğrudan SQL değil):** bugüne göre geçmiş tarihli iki poliçe — `TERM_START_IN_PAST` kontrolü yalnızca API'de, seed `policy.Issue` yolunu doğrudan kullanabilir:
  - **Gecikmiş-A:** başlangıç bugün − 75 gün, 3 taksit, hiç ödeme yok → gün sonu işi 30 gün kuralıyla iptal etmeli.
  - **Gecikmiş-B:** başlangıç bugün − 20 gün, 3 taksit, hiç ödeme yok → faiz tahakkuk etmeli, iptal olmamalı.
  - Seed tekrar çalışınca bozulmamalı (sabit `policy_no` ile kontrol). Tarihlerin bugüne göre olduğu `TESTING.md`'de yazsın.

## 3. Uç noktalar

- `GET /api/v1/policies/{id}/billing` — plan, taksitler (gecikme günü dahil), faiz kayıtları, ödemeler + tahsisler + iadeler, açık bakiye. Erişim: poliçe kapsam kuralı (CUSTOMER/AGENT kendi, ACCOUNTING/UNDERWRITER tümü).
- `POST /api/v1/policies/{id}/payments` — ACCOUNTING ve CUSTOMER (kendi poliçesi). `Idempotency-Key` zorunlu. Gövde: `amount_kurus`, `method`; ACCOUNTING isteğe bağlı `payment_date` (≤ bugün ve ≥ o poliçenin son faiz tahakkuku `to`'su, değilse 422 `PAYMENT_DATE_BEFORE_ACCRUAL`); CUSTOMER için bugün. Transaction: poliçe kilidi → o tarihe kadar faiz tahakkuku → tahsis → `payments` + `payment_allocations` + taksit/faiz `paid` ve durumları → yevmiye → audit. İptal edilmiş poliçede de açık bakiye kadar ödeme alınır.
- `POST /api/v1/payments/{id}/refund` — ACCOUNTING. `Idempotency-Key` zorunlu. Gövde: `amount_kurus`, `reason`. Yalnızca `ACTIVE` poliçede (iptal edilmişte 409 — bilinen sınırlama). Tutar ≤ o ödemenin iade edilebilir kalanı. Ödemenin tahsislerini **en son yapılan tahsisten geriye** geri al (taksit/faiz `paid` azalır, durum `PARTIAL`/`PENDING`'e döner), `REFUND` ödemesi + tahsisleri (pozitif tutar, yön `kind`'dan) + yevmiye + audit. Geri açılan taksite faiz ancak sonraki tahakkukta işler.
- `POST /api/v1/billing/run` — ACCOUNTING. Gövde: isteğe bağlı `as_of` (≤ bugün; ileri tarih 422). "Gün sonu işi": 
  1. Açık taksiti olan poliçeler için faiz tahakkuku.
  2. 30 gün kuralına uyan `ACTIVE` poliçeleri **A3 iptal akışını servis seviyesinde çağırarak** iptal et: gerekçe `NON_PAYMENT`, etki tarihi `as_of`, gün esaslı. Aktör = isteği yapan muhasebe kullanıcısı; bu uçta yetki ACCOUNTING'dir, A3'teki "gerekçe ↔ rol" kuralı yalnızca kullanıcı kaynaklı iptal uçları içindir (sistem iptalinde atlanır, iş kuralları ve kilit aynen uygulanır). A4'ün `CLAIMS_AFTER_EFFECTIVE_DATE` koruması engellerse o poliçeyi atla ve raporla.
  3. Her poliçe ayrı transaction; aday seçiminde `FOR UPDATE SKIP LOCKED` (kilitli olan atlanır, sonraki çalıştırmada işlenir; yanıtta `skipped`).
  - Doğası gereği idempotent: aynı `as_of` ile ikinci çalıştırma yeni faiz veya iptal üretmez. Yanıt: tahakkuk sayısı/toplamı, iptal edilenler, atlananlar.
  - Otomatik zamanlayıcı yok (bilinen sınırlama; cron ile bu uç çağrılabilir).
- `GET /api/v1/ledger/trial-balance` — ACCOUNTING. Hesap bazında borç/alacak/bakiye, genel toplam, `balanced`, satırsız/dengesiz yevmiye sayısı (0 olmalı) ve `receivable_reconciliation` (`ledger_1200`, `open_installments`, `open_interest`, `matches`).
- `GET /api/v1/journal-entries` — ACCOUNTING, keyset, satırlarıyla (muhasebe ekranı için).

## 4. Testler ve kabul kriterleri

**Entegrasyon (gerçek PostgreSQL):**
- Düzenleme: taksit toplamı = prim, vadeler doğru, `POLICY_ISSUE` yevmiyesi.
- Kısmi ödeme en eski taksite; faiz önce kapanıyor; birden çok taksite yayılıyor; durumlar doğru.
- **İdempotans:** aynı anahtarla tekrar ödeme → aynı yanıt, tek ödeme, tek yevmiye; aynı anahtarla eşzamanlı 5 istek → tek ödeme.
- **Yarış (zorunlu, A4'teki gate tekniğiyle):** farklı anahtarlarla eşzamanlı iki ödeme, toplamları açık bakiyeyi aşıyor → biri 201, diğeri 422 `OVERPAYMENT`; `paid ≤ amount` her yerde. `-count=10 -race`. **Mutasyon:** poliçe kilidini geçici kaldır → test kırılmalı ya da ikinci savunma (`CHECK paid ≤ amount` → 409'a eşlenmiş) yakalamalı; hangisi yakaladı raporla, kilidi geri koy.
- Eşzamanlı iki iade aynı ödemenin iade edilebilir tutarını aşamaz.
- Gün sonu: Gecikmiş-B'de faiz tutarı elle hesapla eşit; ikinci çalıştırma yeni kayıt üretmiyor. Gecikmiş-A `NON_PAYMENT` ile iptal, açık taksitler en geçten azaltılmış. 29/30 gün sınırı (testte tarihleri kurarak).
- Zeyil +/− ve iptal sonrası plan toplamı ve yevmiyeler doğru; fazla ödemeli poliçe iptalinde `REFUND` kayıtları ve nakit iade tutarı doğru.
- **Her senaryonun sonunda:** tüm defter dengede, her yevmiye dengede ve satırlı, mutabakat değişmezi tutuyor (ortak bir test yardımcısı yaz, her testin sonunda çağır).
- RBAC: AGENT ödeme alamaz (403), CUSTOMER başkasının poliçesine ödeyemez (404), yalnızca ACCOUNTING iade yapar ve gün sonunu çalıştırır.

**`make verify`:** `F-3` ve `T-4` satırları gerçek API ile:
- Yeni poliçe (3 taksit) → kısmi ödeme tahsisi → aynı anahtarla tekrar (aynı yanıt, tek kayıt) → aşırı ödeme 422 → iade.
- Farklı anahtarlarla **paralel** iki ödeme (curl arka planda + `wait`) → biri 201, biri 422.
- Gün sonu → Gecikmiş-B'de faiz kayıtlarının **her biri** API'deki `from`/`to`/anapara/oranla awk ile yeniden hesaplanıp eşit çıkıyor (verify farklı günlerde tekrar çalışınca birden çok kayıt olabilir; toplam değil kayıt bazında kontrol), Gecikmiş-A `NON_PAYMENT` iptal (önceki çalıştırmada iptal edildiyse de PASS).
- Trial balance `balanced=true` ve mutabakat `matches=true`.

## 5. Frontend (en küçük hali)

- Poliçe detayında "Ödeme planı": taksitler (durum, gecikme günü), faizler, ödemeler/iadeler, açık bakiye.
- Muhasebe: ödeme al (tutar, yöntem, tarih), ödeme satırında "İade et", "Gün sonu çalıştır" (`as_of`), mizan + mutabakat sayfası, yevmiye listesi.
- Müşteri: kendi planı + "Öde".
- Acente: çevirmede taksit sayısı.
- Yükleniyor/boş/hata, 375 px. Cila yok.

## Dokümanlar

`openapi.yaml`, `DECISIONS.md` (tek kilit, muhasebe tablosu, tahsis sırası, faiz modeli, tek prim ayarı fonksiyonu, iade semantiği, 30 gün kuralı ve etki tarihi, zamanlayıcı yok, kazanılmamış prim yok), `TESTING.md` (F-3/T-4 + kanıt komutları + seed tarih notu), `requests.http`, `ROADMAP.md` kanıt matrisi, `PROGRESS.md`.

## Kapsam dışı

Fraud, SLA, atama, raporlar (A6). Kazanılmamış prim karşılığı, bileşik faiz, iptal edilmiş poliçede manuel iade, otomatik zamanlayıcı. `git push`.
