# UI-1 — Premium arayüz kabul özeti

## Kapsam ve korunan sınırlar

Mevcut altı rolün React ekranlarında ortak lacivert/turkuaz tasarım, navigasyon,
mobil menü, form/tablo/detay düzeni ve durum rozetleri uygulandı.
Yeni iş özelliği, metrik veya filtre eklenmedi. Backend, API/auth/hooks/roles/format,
hesap ve idempotans kuralları, migration, compose, CI, k6 ve simülatör kaynakları korundu.
Main ve çalışan 3000 web/API değiştirilmedi; aday mevcut bağımlılıklarla 4173'te incelenir.
Aday gerçek yerel API'ye bağlanır; işlem butonları demo iş kaydı oluşturabilir.

## Ölçüm kaynağı ve temiz dal eşlemesi

Temiz dal: `ui/premium-arayuz-temiz`, taban: `2fd6ffe`.
Temiz daldaki eşdeğer uygulama kodu: **`f060fca8c2010f125e560ae7d37f40c0899a40e6`**.
Aşağıdaki koşular kaynak dalın `4290dd85ef384ddffd39d963447c45dc96e96cf7`
kodunda yapılmıştır. Temiz dal aynı uygulama kodunu taşır; bu temizlik görevinde
test, npm veya Docker çalıştırılmamıştır. Ham logların tarihsel SHA başlıkları korunur.

| Koşulan komut | Gerçek sonuç | Süre | Temiz dalda eşdeğer kod |
|---|---|---:|---|
| `make test lint build` | PASS; 20 testli Go paket, vet/ESLint/TypeScript/Vite | 10,947 sn | `f060fca` |
| `make test-integration` | PASS; 21 paket; app 92,300 sn; ayrı geçici PostgreSQL | 98,737 sn | `f060fca` |
| `WEB_URL=http://127.0.0.1:4173 make verify` | **64 PASS / 0 FAIL** | 107,206 sn | `f060fca` |

Ham kayıtlar: [test/lint/build](test-lint-build.log), [integration](integration.log),
[verify](verify.log). Verify'ın eski çıktı etiketindeki “:3000” metnine rağmen
bu koşunun gerçek WEB_URL'si 4173'tür. Verify görsel kabulün yerine kullanılmadı.
Önceki seçili kaynak/SSR kontrolü: 17 eski TSX, 115 API/hook çağrısı,
287 olay/key/validasyon/rol niteliği, 146 initializer ve 10 SSR kontrolü PASS.
Bu kontrol genel davranış eşdeğerliği veya RBAC güvenlik testi değildir.

## Responsive ve etkileşim özeti

- 40 rol/menü birleşimi × 1440/1024/768/375 px = **160 ölçüm**.
- Ek 20 detay/giriş/hesap yerleşimi ve 1 mobil menü ölçümü; sayfa taşması yok.
- 375 px'de document=375; B1 tablo kabı client341/scroll780, yalnız kap içinde kaydırma.
- Login hata/toparlanma; skip link; mobil seçim, Escape ve odağın düğmeye dönmesi çalıştı.
- Kasko+CAM 15.774,80 TL; aynı girdilerle tekrar aynı. Konut 1.950 TL, Trafik 9.315 TL.
- AGENT pano tek satır/toplamsız; UW/muhasebe 21 satır/toplamlı. Tarih/sort/boş/hata kontrolleri geçti.
- Poliçe Enter/detay; müşteri/acente mahremiyeti; iç rol sinyal/görev görünümü;
  ekspertizsiz ADJUSTMENT pasifliği ve 7 adımlı 10.000 TL CAM hesabı doğrulandı.
- Ölçümler bütün alt durumların veya yazma iş akışlarının eksiksiz testi değildir.

## Kontrast ve veri koruması özeti

Seçili 9 metin renk çiftinin tamamı ≥4,5:1; ana buton 4,56:1. Tam WCAG denetimi değildir.
Önceki UI kabulünün READ ONLY karşılaştırmasında 100000 poliçe, 7780 hasar,
6727 ekspertiz, 495 işaretli, 46 açık inceleme/46 açık görev aynı; SLA 46→46.
Kayıtlı parmak izi, ekspertiz özeti, volume ve konteyner kimlikleri aynı bulundu;
parmak izi yeniden hesaplanmadı. Verify olağan demo kabul verisi ekledi.
Bu temizlikte veritabanına bağlanılmadı; bu sayılar tarihsel UI kabulünden aktarılır.
README kişisel cevapları korunur. Ara JSON/kontrol/ilk-deneme kayıtları teslim dalında
bulunmaz; yalnız eski yerel `ui/premium-arayuz` yedeğinde kalır.

## Yapılmayanlar ve sınırlar

- Frontend için otomatik test yok; görünüm ve davranış tarayıcıda doğrulandı.
- Gerçek cihaz, ekran okuyucu ve %200 metin büyütme kontrolü yapılmadı.
- Reduced-motion CSS kuralı var; OS tercihi değiştirilmedi. Canlı sarı SLA verisi üretilmedi.
- Bütün yazma formları tarayıcıdan tekrar gönderilmedi; API yazma kabulü verify'dadır.
- Bu temizlikte test/npm/Docker/CI/k6/kaos/simülasyon/reset/temiz kurulum çalıştırılmadı.
- Ana uygulamaya alma ve push ayrı kullanıcı kararıdır; yapılmadı.

## Seçilmiş ekran görüntüleri

[Masaüstü](desktop.png) · [Mobil](mobile.png) · [Müşteri/acente hasar](claim-public-desktop.png) ·
[İç rol hasar](claim-internal-desktop.png) · [Teklif](quote-desktop.png) ·
[Poliçe detayı, mobil](policy-detail-mobile.png)
