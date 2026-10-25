# CURRENT_TASK — Sadece aktif ve onaylı iş

**Aktif aşama:** A7 — Gün sonu ölçeklemesi + simülatör (100.000 poliçe, 5 yıl) + canlılık göstergesi + 6 rol ekranlarının gözden geçirilmesi

**Durum:** ONAYLI. A8'e geçmek için yeni onay gerekir.

**Ajan notu:** A6'yı yapan ajan devam ediyor. Kurallar aynı (`AGENTS.md` §8 dahil).
- Mevcut kalıpları yeniden kullan:
  - `policy.Issue` (`PolicyNoOverride`, `ActorID`),
  - ödeme servisi ve `store`,
  - `claim` servisleri ve `Transition`,
  - `task` ve `fraud`,
  - enjekte saat ve seed kalıbı,
  - `ledger`, `page`, `platform/metrics`.
- Simülatör iş verisini **yalnızca servis koduyla** üretir.
- Refactor yok. Yeni kod A1–A3 kalıbında yazılır.
- `PROGRESS.md`'ye "A7 (ajan: Codex)" başlığı açılır.
- İlk commit: `docs: A7 görev tanımı`.

**Süre kutusu:** ~4 saat.

**Kapı:** 0. bölüm bitince şunlar yeşil olmadan simülatöre geçme:
- `make test`
- entegrasyon testleri
- yarış testleri `-count=10 -race`
- `make verify`

**Kesme sırası** (süre yetmezse önce bunlar kesilir):
1. Canlı mod.
2. Sistem sayfasındaki dakikalık grafik (üst çubuk göstergesi kalır).
3. Rol ekranlarında engelleyici olmayan iyileştirmeler.

Kesilen her madde `YAPILMADI` olarak yazılır. 100k hedefi süre yüzünden tutmazsa varsayılan değeri düşürme; ölçülen değeri `KISMEN` olarak yaz.

## 0. Gün sonu ölçeklemesi (Opus #7)

- **Aday seçimi.**
  - Yalnızca vadesi `as_of`'tan önce olan açık taksiti ya da açık faizi bulunan poliçeler seçilir.
  - İptal adımı yalnızca `ACTIVE` poliçelere uygulanır.
  - Sorgu indeksli olmalı. Yoksa `00009` içinde `installments (due_date) WHERE status IN ('PENDING','PARTIAL')` eklenir.
- **Parçalı çalışma.**
  - İstek: `{as_of, after_policy_id?, limit?}`. `limit` varsayılan 500, en fazla 2000.
  - ~8 saniyelik bütçe dolunca yeni poliçeye başlanmaz.
  - Yanıt: mevcut alanlar + `next_after_policy_id` ve `done`.
  - Aynı `as_of` ile tekrar çalıştırma zaten idempotent.
- **Hata yalıtımı.**
  - Beyaz liste dışındaki poliçe hatası `failed [{policy_id, code}]` listesine yazılır, koşu devam eder.
  - Sistemik hatada (context iptali, bağlantı hatası) koşu durur.
- **Döngü.** UI'daki "Gün sonu çalıştır" düğmesi `done` olana kadar döngüde çalışır ve ilerlemeyi gösterir. Verify de döngüye geçer.
- **Testler.**
  - Küçük `limit` ile 3 parçalık koşu, tek koşuyla aynı sonucu verir.
  - Test kancasıyla bir poliçede hata zorlanır → diğer poliçeler işlenir, hata `failed` listesinde görünür.
  - `after_policy_id` sınırı doğru işler.

## 1. Kararlar (`DECISIONS.md`, ADR-19'dan devam)

- **ADR-19 Simülatör.**
  - İş verisi doğrudan SQL'le değil, servis katmanıyla üretilir. Böylece defter dengesi, 1200 mutabakatı ve tetikleyiciler yapı gereği korunur; fraud sinyalleri gerçek motordan gelir.
  - Hız için çalışan havuzu kullanılır; simülatör oturumlarında `synchronous_commit=off`. Çökmede son işlemler kaybolabilir ama tutarlılık bozulmaz.
  - Hasar evresi tek iş parçacıklıdır, çünkü fraud sayımı ve atama sıraya bağlıdır.
  - Determinizm tanımı: aynı tohum + aynı anchor + taze DB → aynı parmak izi.
- **ADR-20 Demo/simülasyon ayrımı.**
  - Acente, müşteri, atölye, uzman ve plaka uzayları ayrıdır; poliçe numaraları `SIM-` önekli.
  - Demo hesapları ve demo senaryoları etkilenmez.
  - Atama havuzu ortaktır (simülasyon uzmanları da dahil).
  - `make reset-demo` simülasyon verisini de siler.
- **ADR-21 Gün sonu.** Aday seçimi, parçalı çalışma ve hata yalıtımı.
- **ADR-22 Canlılık.** Kaynaklar: `audit_log` zaman damgaları (yalnızca sayılar, içerik yok) ve `simulation_runs`.

## 2. Simülatör

**Çalıştırma ve kayıt**
- **Binary ve compose.**
  - `cmd/simulator`, aynı backend imajında ikinci binary olarak derlenir (root olmayan kullanıcı).
  - Compose'ta `simulator` servisi `profiles: ["sim"]` ile tanımlanır. Böylece varsayılan `docker compose up` ve 5 dakika hedefi etkilenmez.
  - `depends_on: api (service_healthy)`.
  - `make simulate` hedefi eklenir.
- **Env** (`.env.example`'a da eklenir):

  | Değişken | Varsayılan |
  |---|---|
  | `SIM_SEED` | 42 |
  | `SIM_POLICIES` | 100000 |
  | `SIM_YEARS` | 5 |
  | `SIM_WORKERS` | 8 |
  | `SIM_ANCHOR_DATE` | bugün |
  | `SIM_LIVE` | false |

- **`simulation_runs` tablosu (`00009`).**
  - Alanlar: tohum, parametreler, durum, aşama, işlenen/hedef, başlangıç/bitiş, parmak izi, özet JSON.
  - Aynı tohum ve parametrelerle tamamlanmış bir koşu varsa yeniden üretim yapılmaz; durum log'a yazılır, çıkılır.
  - İlerleme yaklaşık her 1.000 işlemde bir güncellenir.

**Referans veri** (doğrudan SQL serbest; iş tablolarına değil)
- **Kayıtlar.**
  - ~20 acente.
  - Müşteriler: çoğu tek poliçeli, bir kısmı çok poliçeli.
  - 30 atölye.
  - 5 eksper + 8 hasar uzmanı.
  - İşlemlerin aktörü olacak, rol başına simülasyon kullanıcıları.
- **Simülasyon kullanıcıları.** Parolaları kullanılamaz (rastgele hash); `is_active` (atama havuzunda yer alırlar).
- **Kimlik alanları.** Açıkça kurgusal ve mevcut doğrulamalara uygun.
- **Plakalar.** Demo ve verify plakalarıyla çakışmayan ayrık bir uzayda üretilir.
- **Tarife.** Anchor − `SIM_YEARS` tarihinden itibaren her ürün için geçerli tarife yoksa `00009`'a kurgusal bir tarihsel sürüm eklenir (v1 faktörleri, taban fiyatlar × 0,85). T-1 testleri bozulmamalı.

**Faz 1 — poliçe ve tahsilat** (paralel; her varlığın RNG'si `SIM_SEED`'den türetilir, bu yüzden paralellik içeriği değiştirmez)
- **Poliçeler.** Ürün dağılımı KASKO %50, TRAFİK %35, KONUT %15. Başlangıç tarihi `[anchor − 5 yıl, anchor − 15 gün]` aralığında. Taksit sayısı 1/3/6/12. Numaralar `SIM-000001…`.
- **Tahsilat.**
  - Her işlemde servis saati olayın tarihine ayarlanır.
  - Vadesi gelen taksitler vadesinde ya da birkaç gün gecikmeyle ödenir. Gecikmede faizi servis tahakkuk ettirir; ödeme tutarı servisten okunan açık bakiyeye göre belirlenir.
  - Son 60 günde vadesi gelen taksitlerin ~%5'i ödenmez. Bunların birkaç düzinesi 30 gün kuralına girer; böylece gün sonunun gerçek işi olur.
- **İptal.** Hasar planı olmayan poliçelerin ~%3'ü, tarihsel saatle, müşteri talebiyle iptal edilir.
- **Zeyil.** Yok.

**Faz 2 — hasar** (tek iş parçacığı, olay zamanı sırasıyla)
- **Üretim.**
  - Yıllık sıklık: KASKO %12, TRAFİK %6, KONUT %4.
  - Hasar tarihi poliçe döneminde ve ≤ anchor.
  - İşlemleri uygun roldeki simülasyon kullanıcısı yapar; servislerin yetki ve kapsam kuralları atlanmaz.
- **Yaşam döngüsü** (servislerle): bildirim → triaj → ekspertiz → ayar → onay → ödeme → kapanış.
  - ~%8 red.
  - Geçişler arasında rastgele gün farkı var; bir kısmı SLA'yı aşar.
  - Son 30 gündeki hasarlar farklı açık aşamalarda bırakılır.
- **Kasıtlı şüpheli kalıplar.**
  - Başlangıçtan sonraki 15 gün içinde hasar.
  - Yılda ≥ 3 hasarlı plakalar.
  - Atölye patlamaları (30 günde ≥ 5 hasar).

  Skorlar gerçek fraud motorundan gelir. İşaretli tarihsel dosyalar incelenip kapatılır (çoğu red); son 30 gündekiler kuyrukta kalır.

**Faz 3 — özet ve parmak izi**
- **Özet.**
  - Poliçe sayıları (ürün ve durum bazında).
  - Taksit durumları ve ödemeler.
  - Hasar durumları ve işaretli dosya sayısı.
  - Açık ve ihlalli görevler.
  - Yazılan prim, tahsilat, tazminat.
  - Mizan dengesi ve 1200 mutabakatı.
- **Parmak izi.** İş özniteliklerinin sıralı SHA-256'sı. Surrogate id, hasar no ve sistem zaman damgaları hariç.
- **Sistem zaman damgaları** (`created_at`, `reported_at` gibi now() varsayılanlı alanlar). Planında ikisinden birini öner:
  - servis saatine bağlamak,
  - üretim sonunda tek UPDATE ile tarihselleştirmek.

  `audit_log`'a dokunma.

**Canlı mod** (`SIM_LIVE=true`)
- Yaklaşık 5 saniyede bir, gerçek saatle poliçe düzenleme veya tahsilat yapar.
- Hasar üretmez; kuyruk ve atama kontrollerini oynatmasın.
- Deterministik değildir (belgelenir).
- `simulation_runs.status = LIVE` iken `make verify` açık bir mesajla, başlamadan durur.

**Testler** (entegrasyon, küçük N; ör. 1.000 poliçe)
- İki taze DB'de aynı tohum → aynı parmak izi. Farklı tohum → farklı parmak izi.
- Koşu sonrasında mizan dengede, 1200 mutabakatı tutuyor, her hasar geçmişi `claim_transitions`'a uygun ve en az bir işaretli dosya var.
- Demo müşteri, acente, atölye ve kullanıcılarına bağlı simülasyon kaydı yok; plaka uzayları çakışmıyor.
- Tamamlanmış bir koşu tekrar çalıştırılınca yeni kayıt üretmez.

**Gerçek ölçekte kabul**
- `make simulate` (100k) çalıştırılır. Süre, DB boyutu ve özet `PROGRESS.md`/`TESTING.md`'ye yazılır.
- Ardından `make verify` çalıştırılır:
  - tüm satırlar PASS,
  - süre 15 dakikanın altında (süre raporda),
  - F-7 raporu 5 yıla yayılıyor,
  - fraud kuyruğunda simülasyon dosyaları var,
  - gün sonu birden çok parçada tamamlanıyor.

## 3. Canlılık

- **`GET /api/v1/system/activity`** (giriş yapmış tüm roller). Yanıt alanları:
  - `db_ok`, `last_event_at`,
  - son 30 dakikanın dakikalık olay sayıları (`audit_log`, yalnızca sayı),
  - `simulation {status, phase, done, target, seed, fingerprint, started_at, finished_at}`.

  Gerekirse `00009`'a `audit_log (created_at)` indeksi eklenir.
- **Üst çubuk göstergesi** (tüm roller): yeşil/kırmızı nokta + "Son olay: 12 sn önce". 15 saniyede bir yoklama; bağlantı hatası durumu da gösterilir.
- **Sistem sayfası:** dakikalık olay çubukları, simülasyon ilerleme çubuğu ve özet.
- **Yeni verify satırları:**
  - `SYS canlilik`: `db_ok` doğru; verify'ın kendi işlemlerinden sonra `last_event_at` güncel.
  - `T-4 gun-sonu-parcali`: küçük `limit` ile en az 2 parça, sonunda `done`.

## 4. 6 rol ekranlarının gözden geçirilmesi

- Her rolün `TESTING.md`'deki uçtan uca akışı tarayıcıda, 375 px'te ve simülasyon verisi yüklüyken denenir. Kontrol edilenler:
  - yükleniyor/boş/hata durumları,
  - menüden erişim,
  - gerçek API kullanımı.
- Büyük listeler keyset ile sınırlı sayfalarda gösterilir; tam tablo sayımı yapılmaz.
- Bitiş tarihi geçmiş `ACTIVE` poliçeye UI'da "Süresi doldu" etiketi eklenir; durum değişmez.
- A6'da çalıştırılmayan sarı SLA rozeti senaryosu burada denenir.
- Bulunan sorunlar en küçük düzeltmeyle giderilir; yeniden tasarım yok.
- Çıktı: `TESTING.md`'de rol × akış × sonuç tablosu (PASS / FAIL / YAPILMADI).

## Dokümanlar

- `openapi.yaml`
- `DECISIONS.md` (ADR-19…22)
- `TESTING.md`: simülatörün çalıştırılması, ölçümler, rol tablosu, canlı modun verify öncesi durdurulması
- `requests.http`
- `.env.example` (`SIM_*`)
- `ROADMAP.md` A7 kutuları
- `PROGRESS.md`

## Kapsam dışı

- Simülasyonda zeyil.
- `EXPIRED` durum geçişi.
- k6, CI, kaos, EXPLAIN ve README: A8'e kalıyor. EXPLAIN ve k6 ölçümleri simülasyon verisiyle yapılacak.
- A6'nın bağımsız incelemesi: ayrı tur.
- `git push`.
