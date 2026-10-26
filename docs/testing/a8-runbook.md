# A8 kabul çalıştırma kaydı

Bu dosya yöntem/işlem sırasıdır; sonuçlar PROGRESS/TESTING/README ve ham artifact'lara eklenir. Ölçülmeden PASS yazılmaz.

## Veri kapısı

1. Eski100k üzerinde00011/12 uygulandı: [önce/sonra](../a8-migration-preservation.json). SIMpoliçe100000, v0referansı63742 ve sürüm primi72130792135kuruş değişmedi. Eski olay limitleri NULL.
2. Son uygulama kodu4155bae, üretici4, anchor2026-10-25,seed42,5y,16worker,48h. Kullanıcı onaylı reset ve tek100k.
3. Batch tamamlanmadan Docker işlemi/test/verify/yük/kaos yok; çalışma günlüğü/tmp/gp-a8-100k.log.
4. Tamamlandıktan sonra simulation_runs.status/phase/processed/target, summary, fingerprint, süre ve DBboyutu saklanır. Sayısal kapı: açıkFRAUD_REVIEW onlar düzeyinde, açık görev ihlali<%50, DONEAPPRAISAL/eksper uyuşmazlığı0, defter/mutabakat true. Pencere dışı açık görev0.
5. Bu fotoğraftan sonra verify/k6/kaos kendi demo kabul kayıtlarını oluşturur;100k batch summary'si canlı toplam diye sunulmaz.

## Sıralı ölçümler

- `/usr/bin/time -p make verify` → tam çıktı ve exitstatus. T-3olay-limiti satırı gerçek CAM950000kuruş kırpılmayı doğrular.
- `/usr/bin/time -p make load` →20VU3dk, endpoint özel metrikler, JSON/console. İlk FAIL varsa sakla; eşik değiştirme. K6.2.2.0digest sabit.
- `/usr/bin/time -p make chaos` → a/b/c, iyileşme süresi, APIrestartcount, teködeme/yevmiye/1200.
- `make explain` → aynı SQL/parametre, indeksli ve transactionrollback içinde indekssiz; refund/report/workshop üç aday hamçıktıları, üç plan medyanı. En anlamlı ikinci indeks gerekçesi gerçek plana dayanır.
- Pricingaltın testleri, kapsam çıktısı ve test sayısı güncel kaydedilir. Kanıtı tekrarlamak dışında gereksiz tam test döngüsü yapılmaz.
- `gh run list/view` → aynı pushlanmış kodun beş iş sonucu/linki. Kullanıcı pushladı; ajan push yapmaz.

## Temiz kurulum ve disk koruması

Önce host boşalanı≥8GiB ve Docker diskalanı kontrol edilir. Altındaysa işlem durdurulur, kullanıcıya raporlanır; kendiliğinden başka dosya/volume silinmez.

- HEAD ve origin/main SHAaynı olmalı; değilse kullanıcı pushu beklenir.
- Ana yığın `docker compose down` ile durur; **-v kullanılmaz**, guvenpolice_pgdata korunur.
- Yalnız guvenpolice-api/web proje imaj etiketleri kaldırılır; başka projenin kullandığı imaj atlanır ve kaydedilir. Global system/image/builder prune yok.
- Boş, yalnız bu ölçüme ait docker-container buildx builder. Paylaşılan default buildcache'i projeye güvenli ayırmak mümkün değil; ölçümde kullanılmaz ve silinmez. Paylaşılan postgres:16-alpine imajı cache istisnası olarak raporlanır.
- Yeni klasöre aynıSHA clone; proje adı guvenpolice-a8-clean. Varsayılan3000/8080portlarını ana yığın kapalıyken kullanır; DB hostportu yok.
- `cp .env.example .env` başlangıcından `docker compose -p guvenpolice-a8-clean up --build -d --wait` bitişine gerçek süre; hedef≤5dk. Taze `make verify`.
- Çalışan uygulama kodu aynıSHA ise clone api/web imajları ana proje etiketlerine alınabilir; ana100kvolume'a taze seed yazılmaz (seed varolan kullanıcıda noop).
- **Yalnız clone projesi** `docker compose -p guvenpolice-a8-clean down -v` ile temizlenir. Yalnız ölçüme ait builder kaldırılır; clone dosyaları kanıt için kalabilir.
- Ana yığın --no-build ile açılır; SIM100000 korunduğu SQLile doğrulanır; son `make verify`. Sağlık, rol UI, gitstatus ve yerelcommit raporu.

## Teslim yetkisi

README1/4kişisel yanıtı Enes'in kendi metnidir. Public ayarı, push ve anket Enes'e aittir. README/TESTING gerçek FAIL/çalıştırılmadı kayıtlarını saklar. Bonus ve kesilebilir okunabilirlik yapılmadıysa açıkça yazılır.
