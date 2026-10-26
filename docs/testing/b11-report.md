# B1.1 — Bağımsız inceleme düzeltmeleri kapanışı

## 1. Durum

TAMAMLANDI; yalnız bonus/acente-panosu dalı. Kod önce `c374d9b6f96dabd180a0d655f85dfb7452064e87` commit'ine alındı; testler ve ölçümler bu gerçek SHA üzerinde daha sonra çalıştırıldı. Kanıtlar ayrı docs commit'ine alınır; kendi hash'i final rapordadır. Başlangıç 4c52c39; main hâlâ 4d27405. Main'e dokunulmadı, push yapılmadı; merge/rebase/reset yok. 19:00 TRT sınırından önce tamamlandı.

## 2. Uygulanan düzeltmeler

- Ö1: TestProductionReportGoldenLedgerPeriodsAndGroups dört dönem döngüsünde ADR-18 group_by=agent satırlarını panoyla anahtar, etiket ve dört metrikte birebir eşitler. Pano anahtarları tekil, ek satırlar policy_count/premium/claims=0 ve oran=nil olmalı. Toplam eşitliği ayrıca korunur. Böylece daha önce gerçek servislerle üretilen ve altın değerleri doğrulanan zeyil/iptal/onay fixture'ı acente satırlarını da kanıtlar.
- Birim test yorumu, README ve ADR-27: hazır agregatlarda toplam/oran testi ile SQL/işlemleri sınayan entegrasyon ayrıldı; birim teste SQL kanıtı yüklenmiyor.
- scripts/check-agent-performance.py eklendi. Standart Python3 + mevcut Docker profil sorgusuyla üç dönemde tüm satır/toplam ve AGENT izolasyonu kontrol edilir. Varsayılan pano yanıtının sınırları ADR-18'e aynen aktarılır. Parola/token yalnız bellekte, kayıt dışı; kaynak temizliği ve gerçek HEAD kaydedilir. Betik bir kez 100k üzerinde çalıştırılıp b1-http-consistency.json ve üç ham yanıt yenilendi; hepsinde tested_commit c374d9b vardır.
- Ö2: ROADMAP/TESTING durumu B1/ADR-27 olarak düzeltildi; PDF/fotoğraf yok. Başka güncel eksik pano bildirimi bulunmadı. Tarihsel açıklamalar da güncel bonus sonucuna bağlandı.
- K2: ilk poliçe/prim üretiminin tarihi başlangıç günüdür; ileri başlangıçlı poliçe başlangıç gününe kadar varsayılan ilk üretime girmez. Zeyil/iptal ve onay kendi entry_date'iyle hesaplanır; ADR-18 sorgusu değişmedi.
- K3: canlı API adı/anahtarı **Acentesiz / unassigned**; test, verify, OpenAPI ve HTTP örnekleri eşitlendi. UI etiketi API'den alır. Eski ham B1 SQL/plan/tarayıcı metadata kanıtları tarihsel olarak korunur; güncel sorgu b11-agent.sql'dedir.

## 3. Grep çıktısı

İlk taramada izlenen 323 dosya, ham proje.md hariç (AGENTS kuralı), aşağıdaki rg örüntüsüyle arandı. Gerçek komut/sonuç [b11-status-grep.txt](b11-status-grep.txt):

```text
pattern: (acente[\s-]+(performans\s+)?panosu|bonuspanosu)[^.\n]*(YAPILMADI|yapılmadı|henüz uygulanmadı)
exit_code=1
No matches.
ROADMAP.md:110: Bonus durumu: acente performans panosu B1’de eklendi (ADR-27).
TESTING.md:549: Acente performans panosu B1’de eklendi (ADR-27).
```

## 4. Test ve ölçümler

Her komut gerçek kod SHA'sı **c374d9b6f96dabd180a0d655f85dfb7452064e87** üzerinde, bu sırayla çalıştı. Logların ilk satırında da tested_commit bulunur.

| Komut | Sonuç | Süre (sn) |
|---|---|---:|
| `make test lint build` | PASS / exit 0 | 9.339 |
| `make test-integration` | PASS / exit 0 | 96.582 |
| `docker compose up --build -d --no-deps --wait api web` | PASS / exit 0 | 34.901 |
| `make verify` | 64 PASS / 0 FAIL | 116.196 |
| `python3 scripts/check-agent-performance.py` | PASS / exit 0 | 5.487 |

Birimde 20 testli Go paketi, entegrasyonda21 (app88,465sn). Kod commit'inden önce yalnız biçim/sözdizimi ve diff incelemesi yapıldı; test çalıştırılmadı. Bu görevde test/verify FAIL yok. [Gerçek komut/hash/süre kayıtları](b11-validation.json).

AGENT kapsamı, aynı DB'de 100000 SIM ve mevcut demo/kabul kayıtlarıyla, diğer test/verify/simülasyon yükü yokken READ ONLY olarak ölçüldü. Yeni indeks/migration yok:

| Dönem | Üç SQL süresi (ms) | Medyan (ms) |
|---|---|---:|
| all-time | 9.375 / 3.090 / 1.785 | 3.090 |
| default | 1.650 / 1.471 / 1.525 | 1.525 |

[SQL ve ham plan/özet](../explain/b11-agent.json), tested_commit c374d9b. Tekil isteğe/yüke üst sınır garantisi değildir. Güncel HTTP medyanları tüm zamanlar461,028ms /son12ay259,123ms /dar146,023ms; dört metrikli satır/toplam ve ek sıfır satır kontrolü üç dönemde PASS. [Yenilenen kanıt](b1-http-consistency.json).

## 5. 100k koruması ve sınırlar

Başlangıç kabulü eski 4c52c39 tabanında, bitiş kabulü c374d9b sonrasında aynı a8-acceptance.sql ile READ ONLY alındı. [Karşılaştırma](b11-comparison.json): 100000 poliçe/7780 hasar/6727 ekspertiz/495 işaretli/46 açık inceleme/46 açık görev birebir. Kayıtlı parmak izi, ekspertiz MD5'i, DB konteyneri ve volume kimliği aynı; fingerprint yeniden hesaplanmadı. Anlık SLA46→46, azalma yok. Diğer bütün sabit kabul alanları aynı; yalnız ölçüm alanları:

- measured_at: `2026-10-26T14:16:20.463946+00:00` → `2026-10-26T14:32:12.053717+00:00`.
- database_bytes: `2185059351` → `2185296919`.

Verify olağan demo/kabul işlemleri yapar; SIM verisi değişmedi. Simülasyon, reset-demo, volume silme, down -v, prune yok. Migration, CI, simülatör, k6 ve ADR-18 üretim sorgusu değişmedi. Repo dışındaki UI önizlemesine dokunulmadı. Eski B1 ham kanıtları kendi adlandırması ve zamanıyla korunur; güncel API/arayüz sözleşmesi Acentesiz/unassigned'dır. HTTP dosyasının B1 anındaki sürümü `git show 4c52c39:docs/testing/b1-http-consistency.json` ile görülebilir.

## 6. Değişen dosyalar ve diff

Kod/destek belgeleri commit'i:15 dosya,226 ekleme/31 silme. Kanıt/durum dokümanları ayrı docs commit'indedir. Başlangıç4c52c39 sonrası tam dosya manifesti:

| Tam yol | Amaç |
|---|---|
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/DECISIONS.md](<../../DECISIONS.md>) | Birim/entegrasyon ayrımı, standart adlandırma ve güncel ölçümler. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/process/PROGRESS.md](<../process/PROGRESS.md>) | Tarihsel durum düzeltmesi ve B1.1 kapanışı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/README.md](<../../README.md>) | K2 tarih açıklaması, doğru test kapsamı ve güncel kanıtlar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/process/ROADMAP.md](<../process/ROADMAP.md>) | Açık kullanıcı izniyle bonusun B1/ADR-27 durumu. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/TESTING.md](<../../TESTING.md>) | Durum düzeltmesi, betik kullanımı, gerçek SHA/sonuç/performans. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/agent_performance_integration_test.go](<../../backend/internal/app/agent_performance_integration_test.go>) | Acentesiz/unassigned sözleşmesi ve kapsam testleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/app/report_integration_test.go](<../../backend/internal/app/report_integration_test.go>) | Dört dönemde acente satırı/toplam eşitliği ve ek sıfır satır testi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/report/agents.go](<../../backend/internal/report/agents.go>) | Mevcut raporla aynı acentesiz adı/anahtarı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/backend/internal/report/agents_test.go](<../../backend/internal/report/agents_test.go>) | Birim testin gerçek kapsamı ve yeni standart anahtar. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b11-agent-all-time-1.txt](<../explain/b11-agent-all-time-1.txt>) | Gerçek kod SHA’sıyla AGENT SQL/EXPLAIN veya üç örnek medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b11-agent-all-time-2.txt](<../explain/b11-agent-all-time-2.txt>) | Gerçek kod SHA’sıyla AGENT SQL/EXPLAIN veya üç örnek medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b11-agent-all-time-3.txt](<../explain/b11-agent-all-time-3.txt>) | Gerçek kod SHA’sıyla AGENT SQL/EXPLAIN veya üç örnek medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b11-agent-default-1.txt](<../explain/b11-agent-default-1.txt>) | Gerçek kod SHA’sıyla AGENT SQL/EXPLAIN veya üç örnek medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b11-agent-default-2.txt](<../explain/b11-agent-default-2.txt>) | Gerçek kod SHA’sıyla AGENT SQL/EXPLAIN veya üç örnek medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b11-agent-default-3.txt](<../explain/b11-agent-default-3.txt>) | Gerçek kod SHA’sıyla AGENT SQL/EXPLAIN veya üç örnek medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b11-agent.json](<../explain/b11-agent.json>) | Gerçek kod SHA’sıyla AGENT SQL/EXPLAIN veya üç örnek medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/explain/b11-agent.sql](<../explain/b11-agent.sql>) | Gerçek kod SHA’sıyla AGENT SQL/EXPLAIN veya üç örnek medyan kaydı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/a82-final-audit.md](<a82-final-audit.md>) | Tarihsel bonus ifadesini güncel B1 sonucuna bağla. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-http-all-time-raw.json](<b1-http-all-time-raw.json>) | Eklenen betikle bu kod commit’inde yenilenen HTTP satır/toplam kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-http-consistency.json](<b1-http-consistency.json>) | Eklenen betikle bu kod commit’inde yenilenen HTTP satır/toplam kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-http-default-raw.json](<b1-http-default-raw.json>) | Eklenen betikle bu kod commit’inde yenilenen HTTP satır/toplam kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-http-narrow-raw.json](<b1-http-narrow-raw.json>) | Eklenen betikle bu kod commit’inde yenilenen HTTP satır/toplam kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b1-report.md](<b1-report.md>) | Güncel adlandırma ve tarihsel HTTP kanıtının Git sürümü. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-acceptance-after.json](<b11-acceptance-after.json>) | Grep, süre, SHA veya salt okunur önce/sonra koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-acceptance-before.json](<b11-acceptance-before.json>) | Grep, süre, SHA veya salt okunur önce/sonra koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-c374d9b-http-proof.log](<b11-c374d9b-http-proof.log>) | Komut, gerçek tested_commit ve değiştirilmemiş çıktı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-c374d9b-integration.log](<b11-c374d9b-integration.log>) | Komut, gerçek tested_commit ve değiştirilmemiş çıktı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-c374d9b-services.log](<b11-c374d9b-services.log>) | Komut, gerçek tested_commit ve değiştirilmemiş çıktı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-c374d9b-test-lint-build.log](<b11-c374d9b-test-lint-build.log>) | Komut, gerçek tested_commit ve değiştirilmemiş çıktı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-c374d9b-verify.log](<b11-c374d9b-verify.log>) | Komut, gerçek tested_commit ve değiştirilmemiş çıktı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-comparison.json](<b11-comparison.json>) | Grep, süre, SHA veya salt okunur önce/sonra koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-preservation-after.json](<b11-preservation-after.json>) | Grep, süre, SHA veya salt okunur önce/sonra koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-preservation-before.json](<b11-preservation-before.json>) | Grep, süre, SHA veya salt okunur önce/sonra koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-report.md](<b11-report.md>) | Bu kapanış raporu ve tam manifest. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-status-grep.txt](<b11-status-grep.txt>) | Grep, süre, SHA veya salt okunur önce/sonra koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/docs/testing/b11-validation.json](<b11-validation.json>) | Grep, süre, SHA veya salt okunur önce/sonra koruma kanıtı. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/openapi.yaml](<../../openapi.yaml>) | Acentesiz/unassigned sözleşmesi. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/requests.http](<../../requests.http>) | Güncel adlandırma ve kapsam örnekleri. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/check-agent-performance.py](<../../scripts/check-agent-performance.py>) | Tekrar üretilebilir, sırsız ve gerçek SHA kayıtlı HTTP kanıt betiği. |
| [/Users/macair/Okulda yapılan çalışma/guvenpolice/scripts/verify.sh](<../../scripts/verify.sh>) | Standart acentesiz anahtarıyla parametre izolasyonu kontrolü. |

## 7. Sonraki adım

Yalnız kullanıcının merge değerlendirmesi. Main'e dokunulmadı, push yapılmadı; yeni işe başlanmadı.

Son staging taraması: yeni kanıtlar dahil 344 izlenen dosya; exit_code=1, No matches.
