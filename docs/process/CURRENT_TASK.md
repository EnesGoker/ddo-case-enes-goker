# CURRENT_TASK — Sadece aktif ve onaylı iş

**Aktif aşama:** A8 — Teslim hazırlığı: düzeltmeler, CI, yük testi, kaos, EXPLAIN, README, temiz kurulum

**Durum:** ONAYLI. Teslimden önceki son aşama.

**Ajan notu:** A7'yi yapan ajan devam ediyor.
- Kurallar aynı (`docs/process/AGENTS.md` §8 dahil).
- `docs/process/PROGRESS.md`'ye "A8 (ajan: Codex)" başlığı aç.
- İlk commit: `docs: A8 görev tanımı`.
- Push yok; push'u Enes yapar.
- CI sonucunu `gh run list` / `gh run view` ile okuyabiliyorsan oku, okuyamıyorsan Enes'ten iste.
- Bu sabah Opus aynı repoda salt okunur inceleme yapıyor. Bulguları geldikçe 0.B olarak eklenecek.

**Docker kuralı (kritik):** Enes'in başlattığı 100k simülasyon bitene kadar şunlar **yasak**:
- `make up`, `make reset-demo`,
- `docker compose down` / `docker compose restart`,
- entegrasyon testleri, `make verify`, k6.

Bu sürede yalnızca şunlar yapılır:
- kod düzeltmeleri ve birim testleri,
- CI dosyası,
- k6, kaos ve EXPLAIN betikleri,
- README iskeleti.

Simülasyonun bittiğini `docker compose ps` ya da `simulation_runs` durumundan salt okunur olarak anla.

**26 Ekim 08:28 güncellemesi:** Enes'in başlattığı 100k koşusu COMPLETED; süre 29 dakika, balanced ve receivable_matches true. Docker serbest. Aşağıdaki düzeltmelerden sonra başlatılacak yeni 100k koşusu sırasında yine yalnız Docker gerektirmeyen işlere geçilir.

**Saat hedefleri:**

| Saat | Bitmiş olacaklar |
|---|---|
| 12:00 | 0. bölüm, k6, kaos, EXPLAIN |
| 15:00 | CI yeşil, README taslağı, temiz kurulum ölçümü |
| 18:00 | Her şey; son verify |
| 22:30 | Teslim |

**Kesme sırası:**
1. Okunabilirlik turu.
2. Bonus acente panosu. Yalnızca her şey bitip 18:00'den önce vakit kalırsa yapılır.
3. Kaosun (c) adımı. (a) ve (b) kalır.

Kesilen her madde `YAPILMADI` olarak yazılır.

## 0. Düzeltmeler

**0.A — Koordinatör bulguları**

1. **v0 tarifesi.**
   - Sorun: `00009`'daki v0 `valid_from='0001-01-01'`. Bu, tarifeyi geçmişte sınırsız geçerli yapıyor; "tarife yok" testi de 1. yıldan önceye taşınmış.
   - Düzeltme:
     - Yeni bir migrasyonla (`00011`) v0'ın başlangıcı `2020-01-01` olur. `00009`'a dokunma.
     - Mevcut 100k veri silinmeden uygulanmalı (bütün simülasyon poliçeleri ≥ 2021).
     - "Tarife yok" testi 2019-12-31'e döner.
     - `anchor − SIM_YEARS` 2020-01-01'den önceye düşüyorsa simülatör açık bir hatayla başlamaz.
   - Tarife sürümleri için değişmezlik kuralı ya da tetikleyici varsa, yöntemi planında gerekçelendir.
2. **Test süresi.**
   - Simülatör entegrasyon testleri ayrı bir etikete taşınır (ör. `-tags integration,simulation`) ve `make test-simulation` ile koşar.
   - `make test-integration` yeniden ~5–6 dakikaya iner.
   - CI'da simülatör testleri ayrı iş olur.
3. **Ağır uçlar.**
   - Durum: mizan ve sınırsız dönem raporu 100k veride ~2,7 sn sürüyor, yük altında 10 sn'yi aşıyor.
   - Verify'da bu iki ucun istemci zaman aşımı 30 sn olur. Gerekçe: verify doğruluğu sınar, gecikme k6'da raporlanır.
   - README'de ölçülen süreyle birlikte sınırlama olarak yazılır.
   - Basit ve güvenli bir hızlandırma (ör. kapsayan indeks) EXPLAIN ile kanıtlanırsa uygulanabilir. Kanıtlanamazsa dokunma.

4. **Simülatörün açık dosya penceresi.**
   - Son koşuda 303 FRAUD_REVIEW; 330 açık görevin 314'ü SLA ihlalinde. Sebep son 30 günün açık bırakılması.
   - Pencere varsayılan 48 saat olacak ve parametre yapılacak.
   - Pencere dışındaki işaretli dosyalar da tarihsel olanlar gibi incelenip kapatılacak.

**0.B — Opus incelemesi (kullanıcı tarafından onaylı düzeltmeler)**

1. **#1 YÜKSEK — Hasar yanıtlarında iç inceleme bilgisi.** CUSTOMER/AGENT için bildirim, liste, detay ve geçmişte sistem geçişlerinin notu, FRAUD_REVIEW'a giriş ve FRAUD_REVIEW'dan temize çıkış notu gizlenir. Red gerekçesi (→ REJECTED) gösterilmeye devam eder; UI'da “müşteriye gösterilir” ipucu olur. FRAUD_REVIEW yerine UNDER_REVIEW görünür; liste filtresi UNDER_REVIEW ile çalışır. Kabul: müşteri ve acente yanıt gövdelerinde “fraud” ve “FRAUD_REVIEW” hiç geçmez; iç rollerin yanıtı değişmez.
2. **#3 — İşi yapan görev sahibi.** Görevi kapatan kişi, görev başkasına atanmışsa görevi üstlenir (assigned_to = işlemi yapan); eski atanan audit'e yazılır. Sistem geçişlerinde uygulanmaz. Simülasyon sonrası DONE APPRAISAL görevlerinde assigned_to ≠ ekspertizi yapan sayısı 0 olmalı.
3. **#7 — Redle kapanan görev.** ADJUSTMENT ve FRAUD_REVIEW'dan redle çıkan görev DONE kapanır. CANCELLED yalnız iş yapılmadan kapanan görevler içindir (ör. ekspertizi girilmemiş APPRAISAL). İhlal listesi ve özet aynı tanımı kullanır. ADR-17 güncellenir.
4. **#2 — Canlı mod hatası.** Tamamlanmış koşu FAILED yapılmaz; durum COMPLETED, faz LIVE_FAILED kalır. Run içindeki FAILED güncellemesi yalnız status='RUNNING' iken çalışır.
5. **#4 — Sistem özeti yetkisi.** summary yalnız ACCOUNTING/UNDERWRITER'a döner; diğer roller yalnız canlılık sayılarını görür.
6. **#6 — İş anahtarı içeriği.** request_key tekrarında ödeme/iade kind ve refund_of ile aranır; tutar farklıysa 422. Zeyil tekrarında tür karşılaştırılır.
7. **#5 — Değişmeyen gün sonu.** Hiçbir şey değişmeyen poliçeye audit yazılmaz.
8. **#8 — Verify plakası.** Nanosaniyeden türetilen 3 harf + 4 rakamla geniş plaka uzayı kullanılır.
9. **#9 — Reset öncesi simülatör.** TRUNCATE'ten önce simulator servisi compose ile durdurulur.
10. **#10 — Yarım indeks kurtarma.** 00010 önce DROP INDEX CONCURRENTLY IF EXISTS, ardından IF NOT EXISTS olmadan CREATE yapar. Bu migration yalnız yerelde uygulandı; kullanıcı değiştirmeye izin verdi.
11. **Ekspertiz idempotansı.** Ekspertiz ucu Idempotency-Key alır (mevcut middleware yeterli); UI anahtar gönderir. ADR-14 tablosuna eklenir.

**Uygulama sırası ve kabul:**
a. 0.A ve 0.B 1–4; pencere, görev sahibi ve red kapanışı simülatör sonucundan önce düzeltilir.
b. 1k/10k doğrulaması.
c. reset-demo + 100k simulate (~30 dk); bu sırada CI/betikler/README iskeleti ve 0.B 5–11 gibi Docker gerektirmeyen işler.
d. Yeni veriyle verify, k6, kaos ve EXPLAIN.

100k sonunda açık FRAUD_REVIEW sayısı onlar düzeyinde, açık görevlerin ihlal oranı %50'nin altında olmalı.

**T-3 olay başı limiti — ek plan ONAYLI:** Yeni numaralı migration ile policy_version_coverages.event_limit_kurus (NULL=yok, varsa pozitif kuruş); ürün kataloğu → teklif seçimi → poliçe/zeyil sürümü. Muafiyet → olay limiti → teminat limiti → yıllık limit; calc_steps/OpenAPI/verify güncellenir. Eski kayıtlar NULL kalır; geçmiş tazminat/defter değişmez. Altın örnekler:1.000.000−100.000 muafiyet →800.000 olay →700.000 teminat →600.000 yıllık; yalnız olay400.000; NULL, eşitlik ve MaxInt64.
- Seed/katalogda en az bir teminata kurgusal olay limiti verilir (ör. KASKO CAM), adım demoda görünür.
- Verify'a “T-3 olay-limiti” satırı: olay limitinin kırptığı gerçek onay.
- ADR-10 varsayımı bu kararla değiştirilir.
- Süre kutusu90dk; aşılırsa durulur ve raporlanır.
- **Güncel sıra öncekinin yerine geçer:**1k/10k kabulünü kaydet → T-3 →0.B5–11,CI ve betikler → reset-demo + son kodla tek100k simulate → yeni veriyle verify/k6/kaos/EXPLAIN.100k koşarken yalnız dosya düzenlenir.

## 1. CI (`.github/workflows/ci.yml`)

- **Tetikleyiciler:** push ve pull_request.
- **Sürümler:** Go sürümü `backend/go.mod`'dan; Node 20.
- **İşler:**

  | İş | İçerik |
  |---|---|
  | backend | vet + lint (`make lint` ile aynı) ve `-coverprofile` ile birim testleri. Domain paketlerinin kapsamı `GITHUB_STEP_SUMMARY`'de tablo olarak gösterilir ve artifact olarak yüklenir. Domain paketlerinden biri %70'in altına düşerse iş kırılır. |
  | integration | Postgres 16 servis konteyneriyle entegrasyon paketi; simülatör testleri hariç. |
  | simulation | Simülatör testleri; yalnızca main'e push'ta ve elle tetiklemede. |
  | frontend | `npm ci`, typecheck/lint, build. |
  | docker | api ve web imajlarının build'i; push yok. |

- README'ye CI rozeti eklenir.
- **Kabul:** GitHub'daki yeşil koşunun linki raporda yer alır.

## 2. Yük testi (k6)

- **Çalıştırma:** `make load`, resmi k6 Docker imajıyla çalışan sisteme karşı koşar; yerel kurulum gerekmez.
- **Senaryo:**
  - giriş (token setup'ta alınır),
  - poliçe listesi (keyset),
  - teklif hesabı,
  - ödeme planı okuma,
  - hasar görev listesi,
  - dar dönemli rapor,
  - sağlık ucu,
  - küçük oranda idempotent yazma (benzersiz anahtarla ödeme).
- **Yük:** ~20 VU, 2–3 dakika. Eşikler (thresholds) betik dosyasında tanımlıdır.
- **Çıktı:** her uç için p50/p95/p99, hata oranı ve saniyedeki istek.
- **Ortam:** 100k veride çalıştırılır.
- **README:** sonuç tablosu ve ölçüm ortamı (MacBook Air, Docker Desktop, 100k veri). Sayılar gerçek ölçümden gelir.

## 3. Kaos betiği (`scripts/chaos.sh`, `make chaos`)

- **(a) DB durdurulur.**
  - `/health` db=down'u uygun durum koduyla döner.
  - İş uçları JSON hata döner.
  - API çökmez, yeniden başlamaz.
- **(b) DB başlatılır.** N saniye içinde `/health` ok döner ve iş uçları normale döner.
- **(c) Kesinti sırasında idempotent ödeme.**
  - Kesinti sırasında Idempotency-Key ile gönderilen ödeme başarısız olur.
  - Sistem toparlanınca aynı anahtarla tekrar gönderilir → tek ödeme oluşur, mutabakat tutar.
- **Çıktı:** PASS/FAIL satırları. Sonuç TESTING'e ve README'ye yazılır.

## 4. EXPLAIN karşılaştırması (`scripts/explain.sh`)

- **Yöntem:** 100k veride iki kritik indeks için indeksli ve indekssiz planlar alınır. İndekssiz plan şu kalıpla: `BEGIN; DROP INDEX …; EXPLAIN (ANALYZE, BUFFERS) …; ROLLBACK;`.
- **İndeksler:**
  1. `payments (refund_of_payment_id)` kısmi indeksi, ödeme/iade sorgusunda (dün 310 ms → 0,24 ms).
  2. İkinci indeks şu iki adaydan hangisi daha anlamlı fark gösteriyorsa o seçilir, gerekçesiyle:
     - fraud atölye penceresi: `claims (workshop_id, loss_date)`,
     - F-7 dönem raporu: `journal_entries (entry_date)`.
- **Çıktılar:** Ham çıktılar `docs/explain/` altına konur. README'de özet tablo olur: süre, okunan satır/tampon, plan türü.

## 5. Okunabilirlik turu (Opus #12, kesilebilir)

Yalnızca listelenenler yapılır:
- `claim` paketinde sayısal HTTP kodları → `http.StatusX`,
- fonksiyonlar arasına boş satır,
- tek satırlık uzun SQL'ler → çok satırlı,
- `Convert`'in variadic parametresi,
- testteki kullanılmayan değişken.

Davranış değişmez; ardından tam test turu.

## 6. Dokümanlar

**README** (ölçümler hazır olunca, en sonda yazılır). İçerik:
- **Giriş ve kurulum:**
  - aday adı,
  - kurulum: clone, `.env`, `docker compose up --build`, adresler,
  - demo hesapları: 6 rol + ek uzman hesapları.
- **Mimari ve kararlar:**
  - kısa bir mimari diyagram,
  - teknoloji kararları ve frontend gerekçesi.
- **Doğrulama:** test, verify, load, chaos ve EXPLAIN komutları ile gerçek sonuçları.
- **Simülatör:** nasıl çalıştırılır, süre, özet, işaretli oranı.
- **Sınırlar ve eksikler:**
  - bilinen sınırlamalar,
  - `YAPILMADI` / `KISMEN` listesi,
  - bonuslar: ekspertiz notu mevcut, diğerleri yapılmadı.
- **Yapay zekâ kullanımı:** ajan + bağımsız inceleme süreci.
- **§8'in 5 sorusu:**
  - 1. ve 4. sorular Enes'in kendi metniyle gelecek; yer tutucu bırak.
  - 2. soru: gerçek bir sınırlama/kırılma senaryosu.
  - 3. soru: 4. bölümün sonuçları.
  - 5. soru: prim motorunun doğruluğu için komut ve gerçek çıktı (T-1 altın testleri + verify'ın T-1 satırları).

**Şartname izlenebilirliği:** `docs/process/REQUIREMENTS.md`'deki her zorunlu madde için madde → kanıt → durum tablosu (TESTING'de ya da README ekinde). Opus'un çıkaracağı tablo doğrulanarak temel alınır.

**Son hale getirilecekler:**
- TESTING,
- ROADMAP: kanıt matrisi, A8 kutuları, teslim kontrolü,
- PROGRESS,
- DECISIONS (gerekirse),
- `openapi.yaml` ve `requests.http` güncelliği.

## 7. Temiz kurulum ve son doğrulama

- **Hazırlık:**
  - Disk kontrolü: 8 GB'ın altındaysa dur ve raporla.
  - Ana yığını durdur. 100k volume korunur; `down -v` yok.
- **Temizlik:**
  - Bu projenin imajlarını ve build önbelleğini temizle.
  - Diğer projelerin imajlarına ve volume'larına dokunma; `docker system prune --volumes` yok.
  - Kullanımdaki başka bir projenin imajı silinemiyorsa atla ve README'de belirt.
- **Temiz kurulum:**
  - Push edilmiş HEAD ile aynı commit'i yeni bir klasöre, farklı bir compose proje adıyla clone et.
  - `cp .env.example .env` ardından `docker compose up --build -d --wait`; süreyi ölç. Hedef ≤ 5 dakika; ölçülen süre README'ye yazılır.
  - O kurulumda taze seed ile `make verify` çalıştır.
- **Kapanış:**
  - Yalnızca clone'un compose projesi `down -v` ile kaldırılır.
  - Ana yığın tekrar ayağa kalkar ve 100k veriyle son bir `make verify` çalıştırılır.

## Kapsam dışı

- Acente panosu dışındaki bonuslar (acente panosu da yalnızca vakit kalırsa).
- Yeni özellik, mimari değişiklik, büyük refactor.
- Repo'yu public yapmak, anket, push: Enes yapar.
