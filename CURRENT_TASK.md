# CURRENT_TASK — Sadece aktif ve onaylı iş

**Aktif aşama:** A4 — T-3 + F-4/F-5/F-6: hasar bildirimi, ekspertiz, tazminat, limit tüketimi

**Durum:** ONAYLI. A5'e geçmek için yeni onay gerekir.

**Ajan notu:** Bu aşama A1–A3'ü yazan ajandan farklı bir ajana devredildi. Başlamadan önce `AGENTS.md`, `REQUIREMENTS.md`, `PROGRESS.md`, `DECISIONS.md`, `git log --oneline -30` oku; mevcut paket yapısını (`internal/<alan>/{handler,service,repo}`), `AppError`/hata eşlemesini, `access` kapsam kuralını, `idempotency`, `audit`, `page` yardımcılarını ve test stilini (birim + `integration` tag'li gerçek PostgreSQL testleri, `scripts/test-integration.sh`) **aynen kullan**. Yeniden yazma, refactor yok. `PROGRESS.md`'ye bu aşamanın bölümünü "A4 (ajan: Codex)" başlığıyla yaz.

**Süre kutusu:** ~4 saat üst sınır.

## 0. A3 kararları ve küçük işler (önce)

1. **`policy_versions.endorsement_id` doldurulsun:** `db/migrations/00004_*.sql` ile değişmezlik tetikleyicisine "`endorsement_id` yalnızca bir kez `NULL` → değer" izni ekle; zeyil servisi yeni sürümü ekledikten sonra aynı transaction'da bu alanı doldursun. Up/Down/Up dene. Mevcut tetikleyici testlerine bu durum için test ekle (ikinci kez değiştirme reddedilmeli).
2. Başlangıç gününde iptalde sürümün açık kalması (`version_closed=false`) **kabul**. Bu yüzden A4'te sürüm çözümü yalnızca sürüm aralığına değil, poliçenin `cancel_effective_date` ve `end_date` sınırlarına da bakmalı: A3'teki "tarihte teminat" mantığını (`policy.VersionOn` / coverage sorgusu) **yeniden kullan**, kopyalama.
3. Negatif tutarda sıfırdan uzağa yuvarlama ve `base_version_no` / `STALE_VERSION` **kabul**.
4. **Geriye dönük zeyil koruması:** zeyil veya iptal tarihi, o poliçede `REJECTED` olmayan bir hasarın `loss_date`'inden küçük ya da eşitse 422 `CLAIMS_AFTER_EFFECTIVE_DATE`. Entegrasyon testi ekle.

## 1. Hasar bildirimi ve dosya takibi (F-4)

- `POST /api/v1/claims` — CUSTOMER (kendi poliçesi), AGENT (kendi müşterisinin poliçesi), CLAIMS_ADJUSTER. Alanlar: `policy_id`, `coverage_code`, `loss_date`, `description`, `claimed_amount_kurus`, isteğe bağlı `workshop_id`. `Idempotency-Key` zorunlu.
  - `loss_date` gelecekte olamaz; poliçenin o tarihte teminatı yoksa 422 `NO_COVERAGE_ON_DATE`; seçilen teminat o sürümde yoksa 422 `COVERAGE_NOT_IN_VERSION`.
  - `policy_version_id` sunucuda çözülür (istemciden alınmaz). `plate` sürümdeki `vehicle_plate`'ten kopyalanır.
  - Aynı transaction'da: `claims` (`REPORTED`), `claim_status_history` (`NULL → REPORTED`), audit.
- `GET /api/v1/claims` (keyset, `status` filtresi) ve `GET /api/v1/claims/{id}` (geçmiş, ekspertiz, uzlaşma dahil). Kapsam: CUSTOMER/AGENT kendi, CLAIMS_ADJUSTER/ACCOUNTING/UNDERWRITER tümü, APPRAISER yalnızca ekspertizi kendisine düşen dosyalar (basitçe: `APPRAISAL` durumundakiler + kendi ekspertizlerini girdikleri yeter).
- **Durum geçiş servisi (tek yer):** `Transition(tx, claimID, to, actor, note, expectedRowVersion)`. Claim satırı `FOR UPDATE`; `claim_transitions`'ta izinli değilse 409 `INVALID_TRANSITION`; `row_version` uyuşmazsa 409 `STALE_CLAIM`; `claims.status` + `row_version` + `claim_status_history` + audit aynı transaction. Tüm durum değişiklikleri **yalnızca** bu servisten geçer. T-5 (A6) bunu genişletecek: fraud kuyruğu, SLA, atama **bu aşamada yok**.
- `POST /api/v1/claims/{id}/transition` — CLAIMS_ADJUSTER: `TRIAGED`, `APPRAISAL`, `ADJUSTMENT`, `REJECTED` (gerekçe zorunlu), `CLOSED`.

## 2. Ekspertiz (F-5)

- `POST /api/v1/claims/{id}/appraisals` — APPRAISER, yalnızca dosya `APPRAISAL` durumundayken. `damage_amount_kurus`, `is_total_loss`, `notes`, `inspected_at`. Audit. Birden çok ekspertiz olabilir; tazminatta **en son** ekspertiz kullanılır.

## 3. Tazminat hesabı ve limit tüketimi (T-3, F-6)

**Saf paket** (`internal/settlement`), DB'siz, tam sayı, yarımı yukarı. Girdi: brüt hasar, muafiyet tipi/değeri, teminat kalan limiti, yıllık toplam kalan limit. Sabit sıra, her adım `calc_steps` satırı:

1. Brüt hasar = son ekspertizin `damage_amount_kurus`.
2. Muafiyet: `NONE` → 0; `FIXED` → `min(değer, brüt)`; `PERCENT_BP` → `round_half_up(brüt × bp / 10000)`.
3. Muafiyet sonrası = `brüt − muafiyet` (≥ 0).
4. Teminat limiti: `min(muafiyet sonrası, teminat kalan limiti)`.
5. Yıllık toplam limit: `min(4. adım, yıllık kalan limit)` (yıllık bakiye yoksa bu adım atlanır, `calc_steps`'te belirtilir).
6. Ödenecek = 5. adım. Kırpılan her tutar ve nedeni adımda yazılsın.

Limit modeli (karar, `DECISIONS.md`'ye yaz): `policy_version_coverages.limit_kurus` = o teminatın poliçe dönemi boyunca toplam limiti (`policy_limit_balances`'taki teminat satırı); `annual_limit_kurus` = poliçenin tüm teminatları için yıllık toplam limit (`coverage_code IS NULL` satırı). Olay başına ayrı limit yok (varsayım).

**Uç noktalar:**
- `POST /api/v1/claims/{id}/settlement/preview` — CLAIMS_ADJUSTER. Kaydetmez, adımları döner.
- `POST /api/v1/claims/{id}/settlement/approve` — CLAIMS_ADJUSTER, dosya `ADJUSTMENT` durumunda, `Idempotency-Key` zorunlu. **Tek transaction:**
  1. Claim satırı `FOR UPDATE`.
  2. İlgili iki bakiye satırı (teminat + yıllık) `SELECT … FOR UPDATE`, **her zaman `id` artan sırada** (deadlock önleme).
  3. Kilit altında kalan limitlerle hesap.
  4. Ödenecek 0 ise 422 `LIMIT_EXHAUSTED`, dosya `ADJUSTMENT`'ta kalır.
  5. Koşullu `UPDATE policy_limit_balances SET used_kurus = used_kurus + $x WHERE id = $id AND used_kurus + $x <= limit_kurus` (ikinci savunma hattı; etkilenen satır 0 ise hata).
  6. `claim_settlements` (`APPROVED`, `calc_steps`), `limit_consumptions` (her bakiye için), durum `APPROVED` (geçiş servisi ile), audit.
  7. **Muhasebe:** yevmiye kaydı — borç `5000 Tazminat Giderleri`, alacak `2000 Ödenecek Tazminatlar`, `ref_type='CLAIM_APPROVAL'`.
- `POST /api/v1/claims/{id}/pay` — ACCOUNTING, dosya `APPROVED`, `Idempotency-Key` zorunlu. Settlement `PAID` + `paid_at`, durum `PAID`, yevmiye: borç `2000`, alacak `1000 Banka`, `ref_type='CLAIM_PAYMENT'`, `payment_journal_entry_id` dolar.
- **Küçük ortak yevmiye yardımcısı** (`internal/ledger`): `Post(tx, entry, lines)` — satırsız kaydı reddeder, borç = alacak kontrolünü kodda da yapar (DB tetikleyicisi zaten var). A5 bunu genişletecek; şimdilik minimal.
- **Uzlaşma iptali bu aşamada YOK** (karar): `APPROVED` geri alınamaz. `claim_settlements.CANCELLED` kullanılmaz. Bilinen sınırlama olarak dokümante et.

## 4. Testler ve kabul kriterleri

**Saf `settlement` paketi (birim, kapsam ≥ %70):** en az 4 altın örnek, elle hesap yorumuyla: sabit muafiyet; yüzde muafiyet + yarım kuruş yuvarlama; teminat limitiyle kırpma; yıllık limitle kırpma. Muafiyet > brüt → ödenecek 0.

**Entegrasyon (gerçek PostgreSQL):**
- Sürüm seçimi: zeyil tarihinden bir gün önceki hasar eski sürüme, zeyil günündeki hasar yeni sürüme bağlanıyor; iptal tarihinde/sonrasında hasar 422; başlangıç gününde iptal edilmiş poliçede hasar 422.
- **T-3 yarış testi (zorunlu):** yıllık limitin %80'i kadar iki hasar, farklı `Idempotency-Key` ile **eşzamanlı** onaylanır → toplam `used_kurus ≤ limit`; biri tam, diğeri kalanla kırpılmış ödenir (ya da kalan 0 ise 422). Testi en az 10 kez tekrarla (`-count`).
- **Mutasyon denemesi:** bakiye `FOR UPDATE`'ini geçici kaldır → test kırılmalı (ya da koşullu UPDATE 2. savunmada yakalamalı; hangisinin yakaladığını raporla). Geri koy.
- Durum makinesi: izinsiz geçiş 409, eski `row_version` 409, gerekçesiz red 422.
- Muhasebe: onay ve ödeme sonrası her yevmiye kaydında borç = alacak; tüm defterde toplam borç = toplam alacak.
- İdempotans: aynı anahtarla tekrar onay → aynı yanıt, tek settlement, tek yevmiye.
- RBAC: APPRAISER onaylayamaz, CLAIMS_ADJUSTER ödeyemez, CUSTOMER başkasının poliçesine hasar açamaz (404), ACCOUNTING hasar açamaz (403).
- Geriye dönük zeyil koruması (Bölüm 0.4).

**`make verify`:** `F-4`, `F-5`, `F-6`, `T-3` satırları gerçek API ile: hasar aç → triage → ekspertiz → tazminat önizleme adımları ve beklenen kuruş → onay → ödeme; eşzamanlı iki onayda toplam limit aşılmıyor (curl'leri arka planda paralel çalıştır, `wait`).

## 5. Frontend (en küçük hali)

- Müşteri/acente: poliçe detayından "Hasar bildir" formu; kendi hasar listesi.
- Hasar uzmanı: hasar listesi (durum filtresi), detay (geçmiş zaman çizelgesi), geçiş butonları, tazminat önizleme → onay.
- Eksper: `APPRAISAL` dosyaları listesi + ekspertiz formu.
- Muhasebe: `APPROVED` dosyalar + "Öde".
- Yükleniyor/boş/hata, 375 px. Cila yok.

## Dokümanlar

`openapi.yaml`, `DECISIONS.md` (limit modeli, kilit sırası, uzlaşma iptali yok), `TESTING.md` (F-4/F-5/F-6/T-3 + kanıt komutları), `requests.http`, `ROADMAP.md` kanıt matrisi, `PROGRESS.md`.

## Kapsam dışı

Fraud skoru, inceleme kuyruğu, SLA, uzman ataması (A6). Taksit/tahsilat/prim muhasebesi (A5). Uzlaşma iptali. `git push`.
