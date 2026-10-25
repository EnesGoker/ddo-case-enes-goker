# CURRENT_TASK — Sadece aktif ve onaylı iş

**Aktif aşama:** A6 — A4/A5 inceleme düzeltmeleri + T-5 (fraud skoru, inceleme kuyruğu, SLA, atama) + F-7 (prim üretimi, hasar/prim oranı)

**Durum:** ONAYLI. A7'ye geçmek için yeni onay gerekir.

**Ajan notu:** A5'i yapan ajan devam ediyor. Kurallar aynı:
- `AGENTS.md` (§8 commit mesajı kuralı dahil), `PROGRESS.md` ve `DECISIONS.md`'yi oku.
- Mevcut kalıpları **yeniden kullan**: `claim.Transition` (tek durum noktası), `policy.LoadHeader(…, true)`, `store.Adjust`/`Accrue`, `ledger.Post`, `idempotency`, `audit`, `access`, `page`, `caldate`, `proration.RoundDiv`, `platform/metrics.Registry`, test yardımcıları ve gate yarış testi tekniği.
- Kopyalama ve refactor yok.
- Yeni kod A1–A3 kalıbında yazılır: `http.StatusX` sabitleri, fonksiyonlar arasında boş satır, çok satırlı SQL.
- `PROGRESS.md`'ye "A6 (ajan: Codex)" bölümü yaz.
- İlk commit: `docs: A6 görev tanımı`.

**Süre kutusu:** ~5 saat üst sınır; 0. bölüm en fazla 1,5 saat.

**Kapı:** 0. bölüm bitince şunlar yeşil olmadan T-5'e geçme:
- `make test`
- entegrasyon testleri
- yarış testleri (`-count=10 -race`)
- `make verify`

**Süre aşılırsa kesme sırası:**
1. Frontend'de iş yükü paneli ve rapor gruplama seçimi.
2. `/metrics` gauge'ları.
3. CUSTOMER/AGENT yanıtında fraud alanlarını gizleme.
4. Yetişmeyen frontend sayfaları A7'ye devredilir.

Kesilen maddenin testi ve verify satırı da düşer; raporda ve `PROGRESS.md`'de `YAPILMADI` olarak yaz. Şunlar kesilmez: 0. bölüm, backend testleri ve diğer verify satırları.

## 0. Düzeltmeler (önce bunlar, her madde ayrı commit)

Kaynak: Opus'un A4/A5 incelemesi (numaralar oradaki) ve koordinatör bulguları.

1. **Geriye dönük ek prim (Opus #1, KRİTİK).**
   - `store.Adjust`, `delta > 0` için eşik ve vade tarihi olarak `D = max(etki, bugün)` geçirir.
   - Ek prim, vadesi `≥ D` olan açık taksitlere dağılır. Böyle taksit yoksa vadesi `D` olan yeni taksit açılır.
   - Saf `billing.Adjust` değişmez.
   - Regresyon testleri:
     - (a) Faiziyle tamamen ödenmiş poliçeye 40 gün geriye ek prim girilir → yeni taksitin vadesi bugündür; gün sonu faiz yazmaz ve poliçeyi iptal etmez.
     - (b) Vadesi geçmiş açık taksiti olan poliçede ek prim o taksite eklenmez.
2. **Ayar öncesi faiz (Opus #2).**
   - `store.Adjust` başında, Pay/Refund/Run ile aynı kuralla `Accrue(…, min(etki, bugün))` çağrılır.
   - `to ≤ son tahakkuk to'su` ise çağrı hiçbir şey yapmaz; negatif faiz yok.
   - Faiz oranı tek kaynaktan (config) parametre olarak gelir.
   - Test: aynı senaryoda, aynı gün yapılan underwriter iptali ile gün sonu `NON_PAYMENT` iptali aynı faiz tutarını üretir. Beklenen tutar elle hesaplanıp yorumda yazılır.
   - `payment_integration_test.go` içindeki iptal sonrası testi (~388. satır) faiz tutarını da doğrular.
   - Geriye dönük iptalde, etki tarihinden sonrası için **önceden** tahakkuk etmiş faiz geri alınmaz. Bunu ADR'ye bilinen sınırlama olarak yaz.
3. **Yevmiye tarihi = işlem tarihi (Opus #5 genişletilmiş hali; F-7 buna dayanır).**
   - İşlem günü (bugün) tarihli olacaklar:
     - zeyil/iptal prim ayarı yevmiyesi,
     - otomatik nakit iade (`paid_at` ve `REFUND` yevmiyesi),
     - 2. maddedeki ayar öncesi faiz tahakkukunun yevmiyesi (faiz kaydının `from`/`to` aralığı değişmez).
   - Etki tarihi kayıtta ve açıklamada kalır.
   - Açık iş tarihi taşıyanlar aynen kalır: ödeme `payment_date`, gün sonu `as_of` tarihli.
   - Gerekçe: dönem raporu kapanmış bir dönemi sonradan değiştirmemeli.
   - Test: 10-25'te 10-10 etkili iptal → nakit iade ve yevmiyeler 10-25 tarihli.
   - Bu kural mevcut testleri değiştiriyorsa raporda listele.
4. **Reddedilip kapanan hasar (Opus #3).**
   - `policy/change.go:715` koruması, geçmişinde `REJECTED` olan hasarları hariç tutar: `NOT EXISTS (… claim_status_history … to_status = 'REJECTED')`.
   - `TestClaimsBlockBackdatedPolicyChanges`'e CLOSED adımı eklenir.
5. **Hasar durum makinesi çıkmazı.**
   - Bugünkü durum:
     - `FRAUD_REVIEW` yalnızca `ADJUSTMENT` ya da `REJECTED`'a gidebiliyor.
     - Ekspertiz yalnızca `APPRAISAL` durumunda girilebiliyor.
     - Onay ekspertiz istiyor (`APPRAISAL_REQUIRED`).
   - Sonuç: ekspertizden önce kuyruğa düşüp temize çıkan dosya bir daha onaylanamaz.
   - Düzeltme:
     - `00008` ile `('FRAUD_REVIEW','APPRAISAL')` geçişi eklenir.
     - Ekspertiz yoksa `FRAUD_REVIEW → ADJUSTMENT` 409 `APPRAISAL_REQUIRED` döner.
6. **İdempotans çökme penceresi (`00006`).** Bugün iş transaction'ı commit edildikten sonra yanıt ayrı bir transaction'da saklanıyor. Arada çökme olursa, bayat kayıt devralındığında iş ikinci kez işleniyor.
   - **Şema:** `payments`, `endorsements` ve `claims` tablolarına `request_key TEXT` + `UNIQUE (created_by, request_key) WHERE request_key IS NOT NULL`.
   - **Anahtarın taşınması:** middleware anahtarı context'e koyar, servisler oradan okur.
   - **Anahtarı yazan yollar:**
     - ödeme ve manuel iade (`payments`),
     - zeyil ve iptal (`endorsements`),
     - hasar bildirimi (`claims`).

     Otomatik `REFUND` satırlarında anahtar `NULL` kalır.
   - **Sıra:**
     - Kilit alındıktan hemen sonra, **bütün iş doğrulamalarından önce** (`STALE_VERSION`, `OVERPAYMENT`, durum kontrolleri) `request_key` ile kayıt aranır.
     - Kayıt varsa yan etki olmadan, özgün durum koduyla o kaydın kanonik yanıtı döner.
     - UNIQUE ihlali kilit nedeniyle oluşmamalı; oluşursa 409 `IDEMPOTENCY_CONFLICT`.
   - **Yanıt saklama:** `idempotency_keys.response_body` sütunu `bytea` olur. Normal tekrar bayt bayt aynı döner; anahtar sırası sorunu da kapanır.
   - **Gövde kontrolü:** bayat `IN_PROGRESS` devralınırken `request_hash` karşılaştırılıyor mu, doğrula. Karşılaştırılmıyorsa düzelt: farklı gövde → 422.
   - **Zaten korunan uçlar:** çevirme ve tazminat onay/ödeme, durum kontrolü ya da UNIQUE ile korunuyor; ek iş yok. ADR'ye uç → koruma tablosu yaz.
   - **Test (her yazan yol için):**
     - İstek → idempotency kaydını bayat `IN_PROGRESS`'e çek → aynı anahtarla tekrar → aynı kaynak id, tek kayıt, yevmiye sayısı değişmemiş, mutabakat tutuyor.
     - Aynı anahtar + farklı gövde → 422.
     - Normal tekrar → bayt bayt aynı gövde.
7. **Zeyil bağlantı tetikleyicisi (Opus #6, `00007`).**
   - `00004` dalına iki koşul eklenir: zeyil aynı poliçeye ait olmalı ve bu sürümü üreten zeyil olmalı.
   - Opus'un önerisi: `e.id = NEW.endorsement_id AND e.to_version_id = NEW.id`. Kolon adlarını şemadan doğrula; `policy_id` eşitliğini de ekle.
   - Test: Opus'un iki SQL denemesi hata vermeli (kapalı v1'e kendi iptal zeyli; başka poliçenin zeyli).
8. **Seed (Opus #4 + eksikler).**
   - Tüm seed poliçe primleri `pricing.Calculate` ile tarifeden hesaplanır.
   - Plakalar uygulamanın plaka kuralına uyar (`34DEMO55` geçersiz).
   - Eklenecekler:
     - kurgusal adlı 3 atölye (farklı bölgeler),
     - ikinci eksper `eksper2@guvenpolice.test`,
     - ikinci hasar uzmanı `hasar2@guvenpolice.test` (aynı demo parolası).
   - T-5 senaryosu 3. bölümde.
9. **Küçük düzeltmeler.**
   - **Ödeme tarihi alt sınırı (Opus #10):** `payment_date ≥ least(start_date, düzenleme günü)`, değilse 422 `PAYMENT_DATE_BEFORE_POLICY`. Yalnızca `start_date` sınırı yanlış olur, çünkü başlangıçtan önce yapılan peşin ödeme meşrudur.
   - **Salt okunur uçlar kilit almaz (Opus #11):**
     - GET billing tek bir `REPEATABLE READ READ ONLY` snapshot'ta okur.
     - Zeyil/iptal önizlemeleri kilitsiz çalışır; kayıt yolu zaten kilit + `base_version_no` ile korunuyor.
   - **Verify seed B (Opus #9):** `policy_status` şartı kalkar, yalnızca faiz kayıtları doğrulanır.
   - **Testler (Opus #8):**
     - `TestConcurrentRefunds`'a gate ve mutasyon eklenir: kilidi kaldır → test kırılmalı ya da CHECK yakalamalı. Hangisinin yakaladığını raporla, kilidi geri koy.
     - Negatif zeyil testinde taksit ve nakit iade tutarları doğrulanır.
10. **PROGRESS hash'leri.**
    - `filter-branch` sonrası `PROGRESS.md`'deki eski hash'ler (4749fbd, fc9853e, 7abcdbb, 7890706 vb.) artık geçmişte yok.
    - Her birini commit mesajından eşleyip güncelle; eşlenemeyeni kaldır.

## 1. Kararlar (`DECISIONS.md`, ADR-14'ten devam)

- **ADR-14 — İdempotans çökme penceresi.** İçerik:
  - `request_key` ve ön kontrolün sırası,
  - kanonik yanıt (aynı kaynak ve aynı durum kodu; bayt bayt değil),
  - `bytea` yanıt saklama,
  - uç → koruma tablosu.
- **ADR-15 — Faturalama düzeltmeleri.** İçerik:
  - ek prim vadesi `max(etki, bugün)`,
  - ayar öncesi `Accrue(min(etki, bugün))`,
  - yevmiye tarihi = işlem tarihi,
  - sınırlama: geriye dönük iptalde önceden tahakkuk etmiş faiz geri alınmaz.
- **ADR-16 — Fraud skoru (kurgusal).** İçerik:
  - kurallar, puanlar, eşik ve pencereler,
  - değerlendirme yalnızca bildirimde ve hasar tarihine göre yapılır (deterministik, `now()` yok),
  - eşik üstü dosya otomatik kuyruğa girer,
  - eşzamanlı bildirimlerde sayım READ COMMITTED görüntüsüne göredir; ±1 kaçırma kabul edilir, sinyal danışma amaçlıdır.
- **ADR-17 — Görev, SLA, atama.** İçerik:
  - durum girişinde görev açılır,
  - atama: en az açık görev, eşitlikte küçük id,
  - rol başına advisory kilit,
  - SLA takvim saatiyle hesaplanır (mesai takvimi yok),
  - ihlal tanımı.
- **ADR-18 — Raporlar.** İçerik:
  - yazılan net prim = 4000 hareketi (işlem tarihi),
  - tahakkuk eden tazminat = 5000 hareketi (onay tarihi),
  - oran baz puan olarak, yarımı yukarı yuvarlanır,
  - sınırlama: kazanılmış prim, muallak ve IBNR yok.

## 2. Saf paketler (DB'siz, kapsam ≥ %70)

1. **`internal/fraud`** — `Evaluate(girdi, cfg) Sonuç`. Varsayılan değerler kurgusaldır ve env ile değişir (ör. `FRAUD_THRESHOLD`; tam liste `.env.example`'da).

   | Kod | Koşul | Puan |
   |---|---|---|
   | `EARLY_CLAIM` | `loss_date − policy.start_date < 15` gün | 40 |
   | `REPEAT_PLATE` | aynı plakada, bu dosya hariç, hasar tarihi `[loss − 365, loss]` aralığında ≥ 2 hasar | 35 |
   | `WORKSHOP_CONCENTRATION` | aynı atölyede, bu dosya hariç, hasar tarihi `[loss − 30, loss]` aralığında ≥ 4 hasar | 30 |

   - **Skor:** kuralların toplamı; `fraud_flagged = skor ≥ 60`. Tek sinyal kuyruğa sokmaz, iki sinyal sokar.
   - **Uygulanmayan kurallar:** plakasız (KONUT) dosyada plaka kuralı, atölyesiz dosyada atölye kuralı uygulanmaz.
   - **Plaka:** karşılaştırma normalize biçimde yapılır.
   - **Pencere sınırları:** tek yerde hesaplanır (`Window(loss, gün)`); SQL bu değerleri kullanır.
   - **`detail`:** her sinyal sayıları (gün farkı, eşik, önceki dosya numaraları, pencere) ve Türkçe bir `explanation` metnini taşır.
   - **Altın örnekler:**
     - `EARLY_CLAIM`: 14 gün → var, 15 → yok, 0 → var.
     - Pencere: 365. gün içeride, 366. gün dışarıda.
     - `REPEAT_PLATE`: önceki 1 hasar → yok, 2 → var.
     - `WORKSHOP_CONCENTRATION`: atölyede 3 hasar → yok, 4 → var.
     - Skor: 40 + 35 = 75 işaretli; tek başına 40 işaretsiz; 35 + 30 = 65 işaretli.
     - Özel eşikle tam sınır: skor = eşik → işaretli.
2. **`internal/claimtask`** (saf):
   - `DueAt(oluşturma, tip, cfg)`: `APPRAISAL` 72 sa, `ADJUSTMENT` 48 sa, `FRAUD_REVIEW` 24 sa (kurgusal, env ile değişir).
   - `Breached(görev, şimdi)`:
     - açık görevde `şimdi > sla_due_at` → ihlal,
     - kapanmış görevde `completed_at > sla_due_at` → geç kapanmış.
   - `PickAssignee(yükler)`: en az yük, eşitlikte küçük id; havuz boşsa atamasız.
   - Altın örnekler: eşitlik, boş havuz, tam `sla_due_at` anı (ihlal değil).
3. **`internal/report`** (saf kısım):
   - `LossRatioBP(tazminat, prim) *int64`: yarımı yukarı baz puan; `prim ≤ 0` → `nil`.
   - Altın örnekler: 1/3 → 3333, 2/3 → 6667, 0/x → 0, x/0 → nil.

## 3. Akışlara bağlama ve seed

- **Bildirim (`Report`).** Aynı transaction içinde, `request_key` ön kontrolü ve claim INSERT'inden sonra:
  1. Fraud girdileri toplanır ve `fraud.Evaluate` çağrılır.
  2. `fraud_signals`'a kural başına bir satır yazılır; `claims.fraud_score` ve `fraud_flagged` güncellenir.
  3. İşaretli dosya `Transition` ile `REPORTED → TRIAGED → FRAUD_REVIEW` olur. Bu bir sistem geçişidir:
     - `actor_user_id NULL`,
     - not: `Otomatik: fraud skoru 75 ≥ 60`,
     - audit `New` içinde `auto: true`.

  Yeniden değerlendirme yok; dosyada plaka/atölye güncelleme ucu yok.
- **Görevler `Transition` içinde (tek nokta).**
  - Hedef durum `APPRAISAL`, `ADJUSTMENT` veya `FRAUD_REVIEW` ise aynı tipte görev açılır ve atanır.
  - Kaynak durumun açık görevi kapanır: hedef `REJECTED` ise `CANCELLED`, değilse `DONE`.
  - `Appraise`, açık `APPRAISAL` görevini `DONE` yapar; böylece eksperin SLA'sı kendi işine bağlı kalır.
  - `IN_PROGRESS` durumu ve görev başlatma ucu yok.
  - `Transition` imzası değişecekse en küçük haliyle değişsin.
- **Atama.**
  - Havuz: `APPRAISAL → APPRAISER`; `ADJUSTMENT` ve `FRAUD_REVIEW → CLAIMS_ADJUSTER`. Yalnızca `is_active` kullanıcılar.
  - Yük: kullanıcının açık (`OPEN`/`IN_PROGRESS`) görev sayısı, tüm tipler birlikte.
  - Kilit: sayımdan önce rol başına `pg_advisory_xact_lock(<sabit>, hashtext(rol))`.
  - `created_at` ve `sla_due_at` servis saatinden alınır; böylece seed geçmiş tarihli görev kurabilir.
- **Manuel geçiş.**
  - `ChangeStatus` hedeflerine `FRAUD_REVIEW` eklenir; not zorunludur (422 `FRAUD_REVIEW_REASON_REQUIRED`).
  - 0.5'teki kural geçerlidir.
- **Mevcut akışlar bozulmamalı.**
  - API ile açılan poliçede ilk 15 gündeki her hasar `EARLY_CLAIM` alır; aynı plakadaki 3. hasar işaretlenir.
  - Mevcut test veya verify senaryolarında aynı plakada ≥ 3 hasar açan yer varsa farklı poliçeye taşı.
  - Eşiği ya da env'i değiştirerek susturmak yok.
- **İndeksler (`00008`, yoksa ekle):**
  - `claims (plate, loss_date)`,
  - `claims (workshop_id, loss_date)`,
  - açık görevler için `claim_tasks (assigned_to)` kısmi indeksi,
  - `journal_entries (entry_date)`.
- **Seed senaryosu.** Kurallar:
  - servis koduyla kurulur,
  - sabit `policy_no` sayesinde tekrar çalıştırmaya dayanıklıdır,
  - primler tarifeden gelir,
  - tarihlerin bugüne göre olduğu `TESTING.md`'de yazılır.

  | Adım | Veri | Beklenen |
  |---|---|---|
  | FP-1 | KASKO, plaka P, başlangıç bugün − 300, başlangıç tarihli tam ödeme | faiz yok |
  | FP-1 hasarları | hasar tarihleri bugün − 250 ve bugün − 40 | skor 0, işaretsiz |
  | SLA ihlali | bugün − 40 hasarı, saati bugün − 4 gün olan servisle `APPRAISAL`'a geçirilir | açık `APPRAISAL` görevi, `sla_due_at` geçmiş |
  | FP-1 iptali | müşteri talebi, etki bugün − 6 | — |
  | FP-2 | KASKO, aynı plaka P, başlangıç bugün − 5, tam ödeme | — |
  | Şüpheli hasar | FP-2'de, hasar tarihi bugün − 2 | `EARLY_CLAIM` 40 + `REPEAT_PLATE` 35 = 75 → `FRAUD_REVIEW`; görev, yükü en az hasar uzmanına |

## 4. Uç noktalar

- **`GET /api/v1/claim-tasks`**
  - Erişim: CLAIMS_ADJUSTER tüm görevleri görür; APPRAISER yalnızca kendine atananları (sunucuda zorlanır).
  - Filtreler: `status=open|done|all` (varsayılan `open`), `task_type`, `breached=true`, `claim_id`, `assigned_to=me`. Sayfalama keyset.
  - Satır alanları: `id`, `claim_id`, `claim_no`, `task_type`, `status`, `assigned_to {id, full_name} | null`, `created_at`, `sla_due_at`, `completed_at`, `breached`, `remaining_seconds` (gecikmede negatif).
  - `FRAUD_REVIEW` satırında ek olarak `fraud_score` ve sinyal kodları gelir.
  - **Fraud kuyruğu = `task_type=FRAUD_REVIEW&status=open`.** Ayrı uç yok.
- **`GET /api/v1/claim-tasks/workload`** — CLAIMS_ADJUSTER. Her havuz (APPRAISER, CLAIMS_ADJUSTER) için kullanıcılar ve açık görev sayıları.
- **`GET /api/v1/claim-tasks/sla-summary`** — CLAIMS_ADJUSTER. Görev tipi başına `open`, `open_breached`, `done`, `done_late`, `on_time_bp`.
- **`GET /metrics`**
  - Yeni gauge'lar: `guvenpolice_claim_tasks_open{task_type}` ve `guvenpolice_claim_tasks_sla_breached{task_type}`.
  - `Registry`'ye scrape anında çalışan toplayıcı desteği eklenir: tek sorgu, kısa timeout.
  - Sorgu hata verirse bu metrikler atlanır; uç 500 dönmez.
- **`GET /api/v1/claims/{id}`**
  - CLAIMS_ADJUSTER görür:
    - `fraud {score, threshold, flagged, signals[{rule_code, points, detail, explanation}]}`,
    - `tasks[]`,
    - `allowed_transitions[]` (`claim_transitions` ∩ rolün manuel hedefleri).
  - APPRAISER: `tasks[]`.
  - CUSTOMER/AGENT: hasar yanıtlarında (liste ve detay) `fraud_score`, `fraud_flagged` ve sinyaller yer almaz.
- **`POST /api/v1/claims/{id}/transition`** — 3. bölümdeki ek hedef ve kurallar.
- **`GET /api/v1/reports/production?from=YYYY-MM-DD&to=YYYY-MM-DD&group_by=month|product|agent`**
  - Erişim: ACCOUNTING, UNDERWRITER.
  - Dönem `[from, to)`, yevmiyenin `entry_date`'ine göre. `from` ve `to` isteğe bağlıdır; verilmeyen sınır açık kalır.
  - Biçim hatası veya `from ≥ to` → 422 `INVALID_PERIOD`.
  - Satır alanları:
    - anahtar + etiket,
    - `written_premium_kurus`: 4000 hesabının net alacağı; `POLICY_ISSUE` ve `ENDORSEMENT` ref'leri poliçeye bağlanır,
    - `policy_count`: dönemdeki düzenlemeler,
    - `incurred_claims_kurus`: 5000 hesabının net borcu; A4 onay yevmiyesi, ref → hasar → poliçe,
    - `loss_ratio_bp | null`.
  - Yanıtta ayrıca toplam satırı bulunur.
  - Acentesiz poliçeler `agent` gruplamasında ayrı satırdır; satırlar toplamı her zaman genel toplama eşittir.

## 5. Testler ve kabul kriterleri

**Entegrasyon (gerçek PostgreSQL):**
- **Kurallar.** Her kural tek başına ve birlikte denenir; doğru `fraud_signals` satırları (kod, puan, detay), skor ve bayrak kontrol edilir.
  - İşaretli dosyanın geçmişinde `REPORTED → TRIAGED → FRAUD_REVIEW` (aktör `NULL`) ve atanmış açık bir `FRAUD_REVIEW` görevi bulunur.
  - Pencere sınırı (365. ve 366. gün) bir entegrasyon testinde de doğrulanır.
- **Çıkmaz regresyonu.**
  - Ekspertizsiz `FRAUD_REVIEW → ADJUSTMENT` → 409 `APPRAISAL_REQUIRED`.
  - `→ APPRAISAL → ekspertiz → ADJUSTMENT → onay → ödeme` uçtan uca çalışır.
- **Görev yaşam döngüsü.**
  - Durum girişinde açılır.
  - `Appraise` ile ve durumdan çıkışta kapanır (`REJECTED` → `CANCELLED`).
  - `sla_due_at = created_at + tip süresi`.
  - Havuz boşsa görev atamasız kalır.
- **Yarış (zorunlu, gate tekniğiyle).**
  - Kurulum: 3 hasar uzmanı, eşit başlangıç yükü, 20 eşzamanlı atama (her biri ayrı transaction).
  - Gate rol kilidini tutarken hepsi `pg_stat_activity`'de bekler.
  - Beklenen: yükler 7/7/6; fazlası küçük id'lerde.
  - `-count=10 -race` ile koşulur.
  - **Mutasyon:** advisory kilidi geçici olarak kaldır → test kırılmalı. Sonucu raporla, kilidi geri koy.
- **SLA.** Enjekte saatle ihlal listesi, özet sayıları ve `/metrics` çıktısı doğrulanır.
- **RBAC.**
  - CUSTOMER/AGENT görev ve özet uçlarında 403 alır.
  - APPRAISER başkasının görevini görmez.
  - CUSTOMER/AGENT hasar yanıtında fraud alanı yoktur.
  - AGENT/CUSTOMER rapor ucunda 403 alır.
- **Bildirim çökme penceresi (0.6).** Tekrar istek fraud'u yeniden değerlendirmez (sinyal sayısı aynı kalır) ve ikinci görev açmaz.
- **Rapor altın veri seti.**
  - İçerik: iki ürün; iki acente + acentesiz poliçe; + ve − zeyil; iptal; onaylanmış bir tazminat. Kayıtlar iki ayrı aya düşer (enjekte saat).
  - Beklenen satırlar elle hesaplanıp yorumda yazılır.
  - Her gruplamada: satırlar toplamı = toplam = defterdeki 4000/5000 hareketi.
  - `to` günündeki kayıt dışarıda kalır.
- **Her senaryonun sonunda** A5'in ortak yardımcısı çağrılır: defter dengede, mutabakat tutuyor.

**`make verify`** (`F-7` ve `T-5` satırları gerçek API ile; ayrıca bir `T-4` eki):

| Satır | Kontrol |
|---|---|
| `F-7 prim-delta` | `[bugün, yarın)` raporu alınır → yeni poliçe düzenlenir → toplam tam olarak bu poliçenin primi kadar artar. |
| `F-7 defter-mutabakati` | Sınırsız dönem raporunda yazılan prim = mizandaki 4000 net alacak; tazminat = 5000 net borç. |
| `F-7 boyut-toplami` | Ay, ürün ve acente gruplamalarının her birinde satırlar toplamı = toplam. |
| `F-7 oran` | `loss_ratio_bp`, awk ile hesaplanan `round_div(tazminat × 10000, prim)` değerine eşit. |
| `T-5 seed-supheli` | FP-2'nin sabit `policy_no`'su üzerinden bulunan dosya `FRAUD_REVIEW`'da; skor 75 (`EARLY_CLAIM` 40 + `REPEAT_PLATE` 35); açık ve atanmış görev var. |
| `T-5 esik` | Bugün başlayan, bu çalıştırmaya özgü geçerli plakalı yeni poliçede aynı gün üç hasar: ilk ikisi 40 ve işaretsiz (`REPORTED`), üçüncüsü 75 → `FRAUD_REVIEW`. |
| `T-5 atama` | Üçüncü hasardan önce yükler `workload`'dan alınır; görev en az yüklü uzmana (eşitlikte küçük id) gitmiş olmalı. |
| `T-5 paralel-atama` | Farklı plakalı iki poliçede (poliçe kilidi paralelliği bozmasın) aynı anda işaretli hasar açılır (curl arka planda + `wait`). Atananlar, önceki yüklerden açgözlü sırayla hesaplanan beklenen kümeye eşit olmalı. |
| `T-5 inceleme` | Ekspertizsiz `→ ADJUSTMENT` 409 `APPRAISAL_REQUIRED`; `→ APPRAISAL` 200 ve eksper görevi açılmış. |
| `T-5 sla` | FP-1 dosyasının ihlal görevi `claim_id` filtresiyle `breached=true` dönüyor; `/metrics` çıktısında `guvenpolice_claim_tasks_sla_breached` var. |
| `T-5 yetki` | CUSTOMER görev listesinde 403 alır. |
| `T-4 idem-bayt` | Aynı anahtarla tekrar ödeme yanıtının gövdesi, ilk yanıtla `cmp` ile bayt bayt aynı. |

## 6. Frontend (en küçük hali)

- **Hasar uzmanı:**
  - **Fraud kuyruğu:** skor, sinyal kodları, atanan, SLA rozeti.
  - **Görevler:** tüm görevler, "yalnızca ihlal" filtresi, uzman iş yükü paneli.
- **Eksper:** **Görevlerim** (SLA rozetli).
- **Hasar detayı (uzman görünümü):**
  - fraud paneli: skor/eşik, her sinyalin açıklaması ve puanı,
  - görevler,
  - `allowed_transitions`'tan gelen butonlar.

  Red ve fraud incelemesine gönderme için not zorunlu.
- **SLA rozeti:** ihlal (kırmızı), son %25 (sarı), zamanında (yeşil).
- **Muhasebe ve underwriter — Raporlar sayfası:** from/to, gruplama seçimi, toplam satırlı tablo, TL biçimi, yüzde oran. Tablonun altında tek satırlık not: "Yazılan prim bazlı; kazanılmış prim ve muallak yok (kurgusal model)".
- **Genel:** yükleniyor/boş/hata durumları, 375 px. Cila yok.

## Dokümanlar

- `openapi.yaml`
- `DECISIONS.md`: ADR-14…18
- `TESTING.md`: F-7/T-5 deneme yolları ve kanıt komutları, yeni demo hesapları, seed fraud ve SLA senaryosu, tarih notu
- `requests.http`
- `.env.example`: fraud ve SLA ayarları, kurgusal oldukları notuyla
- `ROADMAP.md`: kanıt matrisi (F-7, T-5) ve A6 kutuları
- `PROGRESS.md`

## Kapsam dışı

- **A7'ye kalanlar:** Opus #7 (gün sonunun ölçeklenmesi ve hatada devam) — simülatör verisiyle birlikte ele alınır.
- **A8'e kalanlar:** Opus #12 (okunabilirlik turu).
- **Yapılmayacaklar:**
  - kazanılmış prim/UPR, muallak ve IBNR,
  - mesai takvimli SLA,
  - manuel atama ve yeniden atama,
  - görev başlatma,
  - fraud yeniden skorlama,
  - acente performans panosu (bonus),
  - geriye dönük iptalde faiz iadesi,
  - simülatör,
  - `git push`.
