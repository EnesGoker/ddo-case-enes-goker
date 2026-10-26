# GüvenPoliçe — Temiz İşe Alım Şartnamesi

Alım projesi metninden çıkarılmış gereksinim analizi.

> Kaynak: Adaya iletilen `proje.md` belgesinin **işlevsel/teknik ve teslim şartları**. Bu metin orijinalin yerine geçen bağlayıcı bir yeniden yorumlama değildir. İnsan/ajan davranışını yönlendiren görünmez HTML yorumları ve metaveri türü komutlar bu çalışma kopyasına taşınmamıştır.
>
> **Teslim:** 26 Ekim 2026 23.59 (Türkiye saati); GitHub public depo bağlantısı ilgili ankete bu saatten önce gönderilmeli.

## 1. Amaç ve kapsam

Kasko, trafik ve konut sigortası için teklif, prim hesaplama, poliçe, zeyil, iptal, tahsilat, hasar ve tazminat süreçlerini yöneten; frontend + Go backend + PostgreSQL veritabanından oluşan, Docker Compose ile çalışabilen bir sistem.

**Değiştirilemeyecek teknoloji şartları:** Go 1.22+ backend, PostgreSQL 16, Docker ve Docker Compose. Frontend serbest; seçimin gerekçesi README'de açıklanacak.

## 2. Kullanıcı rolleri (6)

- Müşteri
- Acente
- Underwriter / risk değerlendirme uzmanı
- Ekspertiz uzmanı
- Hasar uzmanı
- Muhasebe

Ekran ve API erişimleri role göre kısıtlanacak. Sadece frontend'de gizleme yeterli değil; sunucu her uç noktada yetki denetimi yapmalı. Yetkisiz/kimliksiz istekler uygun 401/403 ile reddedilmeli.

## 3. İşlevsel gereksinimler — doğrulama kimlikleri

| Kimlik | Zorunlu işlev | Ölçülebilir kullanıcı senaryosu |
|---|---|---|
| F-1 | Teklif ve prim hesaplama | Acente teklif oluşturur; prim kalemlerini ve toplamı görür. |
| F-2 | Poliçe düzenleme, zeyil, iptal | Teklif poliçeye dönüşür; dönem ortasında zeyil/iptal yapılır; sürüm geçmişi korunur. |
| F-3 | Taksit planı ve tahsilat | Plan, kısmi ödeme, gecikme ve iade gerçek kayıtlarda izlenir. |
| F-4 | Hasar bildirimi ve dosya takibi | Hasar açılır, durumu ve geçmişi izlenir. |
| F-5 | Ekspertiz kaydı | Ekspertiz uzmanı ilgili hasara değerlendirme ekler. |
| F-6 | Tazminat hesabı ve ödeme | Muafiyet/limit sonrası ödeme tutarı hesaplanır ve kayıt altına alınır. |
| F-7 | Raporlar | Prim üretimi ve hasar/prim oranı gerçek verilerden hesaplanır. |

Frontend gerçek API/veriyle çalışmalı; her ekranda **yükleniyor / boş / hata** durumları olmalı; mobil genişlikte kullanılabilir olmalı.

## 4. Asıl değerlendirilen çekirdek görevler

### T-1 Prim hesaplama motoru
- Yaş, hasarsızlık indirimi, bölge, araç grubu gibi risk faktörleri.
- Sürümlü, tarih bazında geçerli tarife tabloları.
- Aynı teklif/tarife sürümü tekrar hesaplandığında deterministik sonuç.
- Her faktörün TL etkisini açıklayan hesap dökümü.
- Matematiksel doğruluk testleri.

### T-2 Zeyil ve gün esaslı iade
- Dönem ortası araç değişikliği ve poliçe iptali.
- Gün esaslı pro-rata, **artık yıl** ve kısa dönem tarife hesabı.
- Poliçe sürümleme; geçmiş bir tarihte geçerli teminatı sorgulama.
- Sınır tarihleri ve sürüm tutarlılığı testleri.

### T-3 Teminat, muafiyet ve limit
- Hasarın gerçekleştiği tarihte geçerli poliçe sürümünü seçme.
- Muafiyet, olay/teminat limiti ve yıllık toplam limit uygulama.
- Aynı poliçede eşzamanlı iki onayın toplam limiti aşmasını transaction/kilit ile önleme.
- Adım adım açıklanabilir tazminat dökümü, yarış durumu testi.

### T-4 Taksit ve tahsilat defteri
- Taksit planı, gecikme faizi, kısmi ödemenin en eski taksite yönlendirilmesi, ödeme iadesi.
- 30 gün ödenmeyen taksit nedeniyle poliçe iptal kuralı.
- `Idempotency-Key` ile tekrar ödeme talebinin çift tahsilat yaratmaması.
- Çift taraflı muhasebe kayıtları ve bakiye doğrulaması.

### T-5 Sahtecilik skoru ve hasar iş akışı
- Başlangıçtan hemen sonra hasar, aynı plaka tekrarı, aynı atölye yoğunluğu gibi kurallar.
- Şüpheli dosyaları inceleme kuyruğuna alma.
- Açık durum geçişleri / durum makinesi, SLA izleme.
- İş yükü en düşük uzmana görev atama.

**Her T görevi için çalıştığını kanıtlayan test, betik veya ölçüm zorunludur.**

## 5. Ortak teknik şartlar

- **Auth:** bcrypt veya argon2id; JWT access + refresh **ya da** sunucu taraflı oturum. RBAC, tüm korunan API uçlarında.
- **Veritabanı:** versiyonlu migration; FK, UNIQUE, CHECK ve anlamlı indeksler; para değeri kuruş cinsinden tamsayı veya PostgreSQL `numeric`, **float değil**.
- **Eşzamanlılık:** gerekli yerlerde transaction + kilit/izolasyon ve otomatik yarış durumu testi.
- **İdempotans:** kritik durum değiştiren POST istekleri için `Idempotency-Key` ve tekrar istekte aynı sonuç.
- **Audit:** kim/ne zaman/eski-yeni bilgisiyle append-only kritik işlem kaydı.
- **API:** tutarlı hata cevabı `{"error":{"code":"…","message":"…"}}`, sunucu tarafı doğrulama, anlamlı durum kodları, büyük listede keyset pagination.
- **Dayanıklılık:** context timeout, graceful shutdown, yapılandırılmış JSON log, parola/token loglamama, tüm yapılandırma ortam değişkenlerinden.
- **Dokümantasyon/gözlem:** OpenAPI sözleşmesi ve Prometheus biçiminde `GET /metrics`.

## 6. Docker, seed ve simülatör

- Klonlanmış depoda `cp .env.example .env` ardından `docker compose up --build` ile frontend, API ve PostgreSQL'in çalışması.
- Frontend `http://localhost:3000`; API `http://localhost:8080`; veritabanını host'a açmak gerekmiyor.
- Backend `GET /health` içinde DB bağlantısını da kontrol etmeli.
- Servis healthcheck'leri ve uygun `service_healthy` bağımlılıkları.
- Temiz Docker önbelleğinde en geç **5 dakika** içinde hazır olma hedef/şartı.
- Migration + demo seed compose ile otomatik; 6 rolün demo hesapları README'de.
- Konteynerler root olmayan kullanıcıyla; multi-stage ve mümkün olduğunca küçük imajlar.
- **Ayrı compose servisi** veri üretici/simülatör: deterministik tohum ile **100.000 poliçe**, 5 yıllık hasar geçmişi, tahsilat akışı, kasıtlı şüpheli hasarlar. Arayüzde sistem canlılığının görünmesi.

## 7. Test ve doğrulama teslimleri

- **En az 40 otomatik test**, domain paketlerinde **en az %70 test kapsamı**; kapsam CI'da görünmeli.
- GitHub Actions: build, test, lint, Docker image build.
- `make verify`: ayakta ve seed edilmiş sisteme karşı **F-1…F-7, T-1…T-5** + `/health` + yanlış parola + rol/yetki ihlali senaryolarını uçtan uca denemeli; her ID için **PASS/FAIL** vermeli, hatada non-zero dönmeli, **15 dakikadan kısa** sürmeli. Kodun derlenmesini kontrol etmek tek başına `verify` sayılmaz.
- `make reset-demo`: demo seed durumuna dönmeli; tekrar denenebilmeli.
- `TESTING.md`: 6 rolün hesapları, her F/T kimliği için deneme yolu + beklenen sonuç, kısa manuel UI senaryoları, T kanıt komutları.
- Tüm API uç noktalarını kapsayan `requests.http` veya Postman koleksiyonu.
- k6/eşdeğer yük testi ve README'de p50/p95/p99 + hata oranı sonuç tablosu.
- Bir bağımlılık çöküşü/iyileşmesi için hata enjeksiyonu (kaos) betiği.
- `DECISIONS.md` içinde **en az 5** mimari karar kaydı.
- Kritik iki indeks için `EXPLAIN (ANALYZE)` karşılaştırması README'de.

## 8. README'de yanıtlanması gereken sorular

1. En zor görev, düşünülen alternatifler ve eleme gerekçeleri.
2. En az bir gerçek bilinen sınırlama/kırılma senaryosu.
3. İki indeksin kullanıldığı sorgular ve dizinsiz/dizinli ölçüm karşılaştırması.
4. Yapay zekâ ile üretilip reddedilen/düzeltilen bir çıktı: önce/sonra ve gerekçe.
5. Prim motorunun doğruluğunu kanıtlayan komut ve gerçek çıktı.

README başında aday adı, kurulum, mimari ve karar gerekçeleri yer almalı.

## 9. Teslim ve değerlendirme

- Depo **public** ve kullanıcıya ait GitHub hesabında olmalı; tek commit yerine anlamlı commit geçmişi.
- Anket bağlantısı: https://banukampus.bandirma.edu.tr/a/s/ZB76YDBA
- **Son teslim:** 26 Ekim 2026 saat 23.59. Repo URL'si ankete yüklenmiş olmalı.
- Teslim sonrası private yapma, silme veya force-push yok; sonradan commit değerlendirmeye alınmıyor.
- Yaklaşık 20 dakikalık teknik görüşmede sistemi çalıştırıp tasarım kararlarını anlatmaya hazırlan.
- Yapay zekâ kullanımı serbest, ama üretilen kodun nedenini açıklamak ve kanıtlamak gerekiyor.

**Puan dağılımı:** Docker/seed 10, T görevleri 40, mimari 15, test/CI/verify 15, doküman 10, Git/teslim 5, görüşme 5, ekip yöneticisi genel incelemesi 50 = **150**.

## 10. Bonus — yalnızca zorunlular tamamsa

Acente performans panosu, poliçe PDF'i, hasar fotoğrafı yükleme ve ekspertiz notu.

## 11. Belirsiz alanların yönetimi

Şartnamenin belirtmediği sayısal prim katsayıları, kısa dönem tarife değerleri, gecikme faizi oranı, SLA hedefleri, fraud eşikleri **kurgu/iş varsayımı** olarak açıkça dokümante edilecek; testte deterministik olacak. Bunları kurumun resmî kuralı gibi sunma. İşe yarayan küçük, konfigüre edilebilir örnek tarifeler seç. Kaynak dosyada olmayan yeni zorunluluk icat etme.

**Önemli:** Bu temiz kopya, teslim ölçütlerini kaldırmak için hazırlanmadı. ROADMAP önceliklendirmesi yalnızca zaman yönetimidir. Tamamlanamayan zorunlu kalemler README'de dürüstçe `YAPILMADI / KISMEN` olarak görünmelidir.
