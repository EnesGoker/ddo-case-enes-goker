# Mimari Karar Kayıtları (taslak)

A0'da onaylanan beş karar. A1'de uygulanan kısımlar işaretlidir; diğerleri ilgili aşamada kanıtla doldurulacak.

## ADR-1 — Modüler monolit, katmanlı paketler

**Karar:** Tek Go binary'si; her iş alanı `internal/<alan>` paketinde handler → service → repo (+ saf domain) katmanlarıyla. Tek React uygulaması, rol bazlı ekranlar.
**Gerekçe:** Teslim süresi kısa; tek kişi/tek ajan; transaction sınırları (limit, tahsilat, defter) aynı veritabanında. Mikroservis dağıtık transaction ve operasyon yükü getirirdi.
**Elenen:** Mikroservis + kuyruk (gereksiz karmaşıklık), ORM (sorgu ve kilit davranışının açık görünmesini istiyoruz).
**Durum:** A1'de uygulandı (`auth`, `customer`, `health`, `platform/*`).

## ADR-2 — Para ve oranlar tam sayı

**Karar:** Para `int64` kuruş (DB `BIGINT`), oranlar `bp` (1/10.000) ve `ppm` (1/1.000.000) tam sayı. Yuvarlama tek noktada, satır bazında, yarımı yukarı; döküm satırları toplamı nihai prime eşit.
**Gerekçe:** Float ikili gösterimi kuruş hatası üretir; tam sayı deterministik ve test edilebilir.
**Durum:** Şema hazır; hesap kodu A2'de.

## ADR-3 — Tarih aralıkları `[başlangıç, bitiş)` ve gerçek gün sayısı

**Karar:** Tüm aralıklar yarı açık. Pro-rata, poliçe döneminin gerçek gün sayısıyla (365/366) hesaplanır; kısa dönem oranı tarife tablosundan.
**Gerekçe:** Sınır günlerinin iki dilime birden düşmesini engeller; PostgreSQL `daterange '[)'` ve EXCLUDE kısıtlarıyla uyumlu.
**Durum:** A3'te test edilecek (29 Şubat, yıl sonu).

## ADR-4 — Eşzamanlılık ve idempotans

**Karar:** Limit tüketimi koşullu `UPDATE ... WHERE used + x <= limit`; taksit/tahsilatta `SELECT ... FOR UPDATE`; kritik POST'larda `Idempotency-Key` (`idempotency_keys`, `IN_PROGRESS` → `COMPLETED`, tekrar istekte kayıtlı yanıt).
**Gerekçe:** Uygulama içi mutex çok replikada çalışmaz; veritabanı tek doğruluk kaynağı.
**Durum:** A1'de aynı desen refresh rotasyonunda uygulandı (`FOR UPDATE`, eşzamanlı 5 istekte yalnızca biri başarılı — entegrasyon testi). Limit/tahsilat A4–A5'te.

## ADR-5 — Kimlik doğrulama: kısa ömürlü JWT + dönen refresh cookie

**Karar:** 15 dk HS256 access token (yalnızca bellekte, `Authorization: Bearer`). Refresh token 256 bit rastgele, DB'de yalnızca SHA-256 özeti; httpOnly + SameSite=Strict cookie, `Path=/api/v1/auth`. Her yenilemede döner; kullanılmış token tekrar gelirse kullanıcının tüm refresh token'ları iptal edilir. Parolalar bcrypt (cost 12). nginx `/api`'yi aynı origin'den API'ye iletir.
**Gerekçe:** XSS ile refresh token çalınamaz; CSRF'e karşı SameSite=Strict + access token'ın header'da olması; stateless erişim denetimi + iptal edilebilir oturum.
**Bedel:** Paralel iki yenileme isteği oturumu düşürür; frontend yenilemeyi tek uçuşa indirir (`refreshSession`). Access token iptal edilemez (15 dk pencere).
**Durum:** A1'de uygulandı ve test edildi.

## Ek notlar (A1)

- Migrasyonlar API açılışında goose ile uygulanır; demo seed idempotenttir ve `DEMO_PASSWORD` değişirse hash'i günceller.
- `/metrics` ek bağımlılık yerine elle yazılmış küçük bir Prometheus metin üreticisidir; etiket olarak ham yol yerine chi rota kalıbı kullanılır (kardinalite).
