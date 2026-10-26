# B1 — Acente performans panosu kapanış raporu

## 1. Durum ve dal

**TAMAMLANDI**, `bonus/acente-panosu`; başlangıç `fb25280` (ilk ölçümde durma kaydı). Önceki B1'in yerine geçen yeni kullanıcı kapsamı uygulandı. Uygulama commit'i `53aceb4`; belge/kanıt commit'inin kendi hash'i final rapordadır. Push çıktısı yok: git push çalıştırılmadı. Main'e push dışında dokunulmadı; push da yapılmadı, main ref'i `4d27405` olarak kaldı. Merge/rebase/reset yok. Kapanış 18:30 TRT sınırından öncedir.

## 2. Metrikler, dönem ve görünürlük

- Poliçe adedi: dönem içindeki POLICY_ISSUE hareketi.
- Yazılan net prim: 4000 net alacak; poliçe + zeyil/iptal. Nakit ödeme/iade yeniden prim üretmez.
- Tahakkuk eden tazminat: CLAIM_APPROVAL kaynaklı 5000 net borç; tazminatın nakit ödenmesi yeniden sayılmaz.
- Yazılan prime göre hasar/prim oranı: mevcut lossratio ile int64 baz puan; prim <=0 ise null/“—”. Genel oran toplamlardan, ortalama satır oranından değil. Toplam int64 taşması hata verir.

Yevmiye entry_date [from,to). Parametreler yokken TRT bugün dahil son 12 takvim ayı: 26 Ekim'de [2025-10-27,2026-10-27); açık boş tarihler tüm zamanlar. Bütün acenteler, hareketsizler sıfır, acentesiz poliçe varsa Doğrudan; toplam en altta. AGENT yalnız kimliğinden çözülen kendi satırı, total yok; parametreyle genişleme yok. UNDERWRITER/ACCOUNTING tümü; diğer roller 403, kimliksiz 401. Mevcut ADR-18 report.go sorgusu değişmedi.

## 3. Performans ve tutarlılık

[Eski plan](../explain/b1-baseline.txt): 3972,337 ms, tek örnek; medyan değildir. Yeni sorgu issue/zeyil/onayı ayrı acente gruplarında toplar; sonra birleştirir. Yeni indeks/migration gerekmedi.

| Dönem | Üç SQL süresi (ms) | Medyan (ms) | Kapı |
|---|---|---:|---|
| Tüm zamanlar | 2113,342 / 501,841 / 443,704 | 501,841 | PASS, <1000 |
| Son 12 ay | 250,441 / 257,549 / 278,072 | 257,549 | Ölçüldü |

İlk yeni örnek 1 sn üstündedir ve saklandı. Üç örnek medyanı kapısı uygulanır; her isteğe/soğuk önbelleğe/yük altında garanti verilmez. [Ham SQL/planlar ve özet](../explain/b1-after.json).

100k SIM + mevcut demo kayıtlarında üç dönem SQL toplamı/satırları ADR-18 ile birebir: [b1-consistency.json](b1-consistency.json). Verify sonrasında gerçek HTTP ile dört metriğin ve hareketli satırların eşitliği ayrıca doğrulandı; farklı ölçüm anlarının demo toplamları birbirine eşit sayılmadı:

| Dönem | HTTP medyan (ms), üçer istek | Dört metrik/satır eşitliği |
|---|---:|---|
| all-time | 432.560 | PASS |
| default | 229.244 | PASS |
| narrow | 56.785 | PASS |

[HTTP ham yanıtları ve kapsam](b1-http-consistency.json). AGENT'ın beklenen kimliği bağımsız DB profilinden kontrol edildi; Doğrudan/total isteği yalnız kendi satırını döndürdü.

## 4. Testler ve tarayıcı

İstenen sıra son kodda:

| Komut | Sonuç | Süre (sn) |
|---|---|---:|
| `make test lint build` | PASS / exit 0 | 7.685 |
| `make test-integration` | PASS / exit 0 | 92.566 |
| `docker compose up --build -d --no-deps --wait api web` | PASS / exit 0 | 13.084 |
| `make verify` | 64 PASS / 0 FAIL | 108.895 |

Birimlerde 20 testli Go paketi; entegrasyonda 21 testli paket (app88,777 sn). Dört yeni unit fonksiyonu altın toplam/oran, projeksiyon/taşma, açık/varsayılan tarihler ve reddedilen rolleri sınar. Yeni entegrasyon kendi/başka acente, profilsiz AGENT, kimliksiz/tüm rol matrisi, parametre saldırısı, nakit tahsilat/iade, hareketsiz/doğrudan ve [) sınırı içerir. Mevcut servis tabanlı altın rapor fixture'ı zeyil/iptal/onay sonrası dört dönem pano eşitliğiyle genişletildi. Sonraki üç dönem ayrı fixture'da da karşılaştırılır. [Komut/süre kayıtları](b1-validation.json).

İlk başarılı komut dizisi de korunur. Tarayıcıda ilk boş tarih denemesi FAIL (b1-ui-first-failure.json): alanlar boşken eski tarihler gönderildi. FormData düzeltmesinden sonra bütün sıra yeniden geçti; tarayıcıda açık boş tarihler tüm zamanları getirdi. Test/FAIL kaydı silinmedi. YAML kontrolü için yerel Python/node modülleri bulunmadı; paket kurmadan sistem Ruby YAML ile belge ve 276 iç referans doğrulandı (b1-doc-check.log).

Gerçek tarayıcı: 375px document=viewport375, tablo kabı341/iç tablo780; yatay kaydırma yalnız tabloda. AGENT1satır/total0, UNDERWRITER21satır/total1; masaüstü1440/1440. Ana veri kümesinde acentesiz poliçe yok; Doğrudan fixture ile sınandı. Sıralama artan/azalan, loading, empty, error/yeniden deneme, sıfır primde “—” ve tarih sıfırlama görüldü. Frontend test deseni yok, test paketi eklenmedi. Viewport sıfırlandı ve kabul sekmesi kapatıldı. [UI kaydı](b1-ui.json), [mobil](b1-ui-mobile.png), [masaüstü](b1-ui-desktop.png).

## 5. 100k koruması ve sınırlar

A8.1 yöntemiyle [karşılaştırma](b1-comparison.json): 100000 SIM poliçe /7780 hasar /6727 ekspertiz /495 işaretli /46 açık inceleme /46 açık görev aynı. Kayıtlı parmak izi ve ekspertiz MD5'i aynı; fingerprint yeniden hesaplanmadı. Volume kimliği ve DB konteyneri korundu. Gerçek saate bağlı SLA 46→46; düşüş yok. Ölçüm alanları:

- measured_at: `2026-10-26T13:17:51.638882+00:00` → `2026-10-26T13:42:16.510547+00:00`.
- database_bytes: `2184715287` → `2185059351`.

Diğer tüm kabul alanları birebir eşit. Verify olağan demo/kabul verisini değiştirir; SIM iş alanları değişmedi. Simülatör, k6, CI, compose ve migration kaynakları değişmedi; yeni 100k, down -v, prune veya volume silme yok. Yük/k6 ve simülasyon testleri bu bonus kapsamında çalıştırılmadı. Repo dışındaki UI önizlemesine dokunulmadı.

## 6. Değişen dosyalar

Tam yollar, fb25280 sonrası bu görevin dosyaları:

| Tam yol | Amaç |
|---|---|
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/DECISIONS.md](<../../DECISIONS.md>) | ADR-27 metrik/dönem/yetki/sorgu kararı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/PROGRESS.md](<../../PROGRESS.md>) | Görev sonucu ve yerel commit/kısıt kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/README.md](<../../README.md>) | Bonus, uç listesi, izlenebilirlik ve gerçek ölçümler. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/TESTING.md](<../../TESTING.md>) | Performans, tutarlılık, son testler, UI ve koruma sonuçları. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/agent_performance_integration_test.go](<../../backend/internal/app/agent_performance_integration_test.go>) | Gerçek DB/HTTP rol matrisi, kapsam, doğrudan/boş, nakit iade ve tarih sınırı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/report_integration_test.go](<../../backend/internal/app/report_integration_test.go>) | Zeyil/iptal/onay fixture’ında pano–ADR-18 eşitliği. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/router.go](<../../backend/internal/app/router.go>) | Yeni GET ucunu izinli üç role bağla. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/report/agents.go](<../../backend/internal/report/agents.go>) | Bağımsız toplama sorgusu, tarih varsayılanı, rol/profil kapsamı ve güvenli toplam. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/report/agents_test.go](<../../backend/internal/report/agents_test.go>) | Altın metrik, projeksiyon, taşma, dönem ve yetki unit testleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b1-after-all-time-1.txt](<../explain/b1-after-all-time-1.txt>) | Kaynak başına toplama sorgusunun ham planı, sorgusu veya medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b1-after-all-time-2.txt](<../explain/b1-after-all-time-2.txt>) | Kaynak başına toplama sorgusunun ham planı, sorgusu veya medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b1-after-all-time-3.txt](<../explain/b1-after-all-time-3.txt>) | Kaynak başına toplama sorgusunun ham planı, sorgusu veya medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b1-after-default-1.txt](<../explain/b1-after-default-1.txt>) | Kaynak başına toplama sorgusunun ham planı, sorgusu veya medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b1-after-default-2.txt](<../explain/b1-after-default-2.txt>) | Kaynak başına toplama sorgusunun ham planı, sorgusu veya medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b1-after-default-3.txt](<../explain/b1-after-default-3.txt>) | Kaynak başına toplama sorgusunun ham planı, sorgusu veya medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b1-after.json](<../explain/b1-after.json>) | Kaynak başına toplama sorgusunun ham planı, sorgusu veya medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b1-after.sql](<../explain/b1-after.sql>) | Kaynak başına toplama sorgusunun ham planı, sorgusu veya medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-acceptance-after.json](<b1-acceptance-after.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-acceptance-before.json](<b1-acceptance-before.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-comparison.json](<b1-comparison.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-consistency-all-time-raw.json](<b1-consistency-all-time-raw.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-consistency-default-raw.json](<b1-consistency-default-raw.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-consistency-narrow-raw.json](<b1-consistency-narrow-raw.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-consistency.json](<b1-consistency.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-doc-check.log](<b1-doc-check.log>) | Gerçek komutun değiştirilmemiş çıktısı veya belge kontrol kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-final-integration.log](<b1-final-integration.log>) | Gerçek komutun değiştirilmemiş çıktısı veya belge kontrol kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-final-services.log](<b1-final-services.log>) | Gerçek komutun değiştirilmemiş çıktısı veya belge kontrol kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-final-test-lint-build.log](<b1-final-test-lint-build.log>) | Gerçek komutun değiştirilmemiş çıktısı veya belge kontrol kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-final-verify.log](<b1-final-verify.log>) | Gerçek komutun değiştirilmemiş çıktısı veya belge kontrol kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-http-all-time-raw.json](<b1-http-all-time-raw.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-http-consistency.json](<b1-http-consistency.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-http-default-raw.json](<b1-http-default-raw.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-http-narrow-raw.json](<b1-http-narrow-raw.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-integration.log](<b1-integration.log>) | Gerçek komutun değiştirilmemiş çıktısı veya belge kontrol kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-preservation-after.json](<b1-preservation-after.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-preservation-before.json](<b1-preservation-before.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-report.md](<b1-report.md>) | Bu kapanış raporu ve tam dosya manifesti. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-services.log](<b1-services.log>) | Gerçek komutun değiştirilmemiş çıktısı veya belge kontrol kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-test-lint-build.log](<b1-test-lint-build.log>) | Gerçek komutun değiştirilmemiş çıktısı veya belge kontrol kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-ui-desktop.png](<b1-ui-desktop.png>) | Gerçek tarayıcıdan görsel kabul kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-ui-first-failure.json](<b1-ui-first-failure.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-ui-mobile.png](<b1-ui-mobile.png>) | Gerçek tarayıcıdan görsel kabul kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-ui.json](<b1-ui.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-validation.json](<b1-validation.json>) | Gerçek önce/sonra, süre, tutarlılık, UI veya koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-verify.log](<b1-verify.log>) | Gerçek komutun değiştirilmemiş çıktısı veya belge kontrol kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/api/types.ts](<../../frontend/src/api/types.ts>) | Rol bazlı total alanı opsiyonel pano tipi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/AgentPerformancePage.tsx](<../../frontend/src/pages/AgentPerformancePage.tsx>) | Dönem, sıralama ve durumları içeren mevcut stilde pano. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/pages/Dashboard.tsx](<../../frontend/src/pages/Dashboard.tsx>) | Yetkili pano görünümünü bağla. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/roles.ts](<../../frontend/src/roles.ts>) | İzinli rollere menü öğesi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/frontend/src/styles.css](<../../frontend/src/styles.css>) | Yatay taşmayı tablo kabında tutan responsive kapsam. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/openapi.yaml](<../../openapi.yaml>) | Yeni uç, dönem ve görünürlük sözleşmesi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/requests.http](<../../requests.http>) | Yetkili/yasak/kimliksiz ve kapsam parametresi örnekleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/verify.sh](<../../scripts/verify.sh>) | Tek B1 rol/tutarlılık PASS/FAIL satırı. |

## 7. Sonraki adım

Yalnız kullanıcının bonus dalını incelemesi ve main'e alıp almama/teslim kararı. Merge, rebase, reset ve push yapılmadı. Yeni göreve başlanmadı.
