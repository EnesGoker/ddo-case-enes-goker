# GüvenPoliçe — Teslime Odaklı Roadmap

**Başlangıç:** 25 Ekim 2026 öğleden sonra. **Son teslim:** 26 Ekim 2026 23.59 (Türkiye saati).

> Yaklaşık **33 saatlik takvim penceresi**, 33 saat kesintisiz verimli kodlama demek değil. Aşağıdaki süreler **agresif zaman kutuları**dır, tamamlanma garantisi değildir. Şartnamenin tamamı normalde çok daha fazla emek ister. Çalışan, kanıtlanmış kısmi teslim; sahte tamamlanmış sistemden daha değerlidir.

## Çalışma yöntemi

- Ajan **tek aşamada** çalışır; bir aşamanın alt görevleri küçük paketlere bölünür.
- Başlamadan `CURRENT_TASK.md` onaylı hale gelir; bitince test + rapor + manuel kontrol yapılır.
- `ROADMAP.md` checkbox'ları yalnızca gözlenen kanıtla tamamlandı yapılır.
- Yeni özellik yerine önce en kritik hata ve başarısız doğrulama giderilir.
- **İlk çalışan dikey akış:** giriş → acente teklifi → faktörlü prim hesabı → poliçe → rol uygun ekranda listeleme.

## İş sırası ve zorunlu geçiş kapıları

### A0 — Kodsuz mimari planı ve kararların kilitlenmesi (hedef 30–60 dakika)
- [x] Tek repo için klasör ağacı, Go/React/Postgres/Compose seçimleri önerildi ve onaylandı.
- [x] Basit modüler monolit mimarisi ve kritik tablo/ilişki taslağı sunuldu.
- [x] İlk auth + teklif + poliçe API sözleşmesinin taslağı sunuldu.
- [x] `DECISIONS.md` için en az 5 kararın konusu belirlendi.
- [x] İlk kodlama görevinin dosya listesi ve testleri kullanıcı tarafından onaylandı.

**Çıkış kapısı:** Kod değiştirilmeden ilk uygulama paketi kesinleşti. **İlk adımda bütün sistemi yazma.**

### A1 — Docker, migration, demo seed, auth ve rol kontrolü (hedef 3 saat)
- [x] `docker compose up --build` frontend:3000, API:8080, DB servislerini ayağa kaldırıyor.
- [x] `/health` DB erişimini kontrol ediyor; servis healthcheck'leri var.
- [x] Migration ve 6 rol demo seed otomatik çalışıyor.
- [x] `Makefile` içinde `make verify` için gerçek API kontrollerine genişletilecek ilk doğrulama altyapısı kuruldu; PASS yalnızca gerçek kontrolle üretiliyor.
- [x] `DECISIONS.md` ve `openapi.yaml` dosyaları oluşturulmaya başlandı.
- [x] Parola hash, oturum/JWT, rol yetkisi sunucuda doğrulanıyor.
- [x] Giriş ve rol bazlı iskelet ekranlar gerçek API'ye bağlanıyor.
- [x] Başarılı/başarısız giriş ve 401/403 için otomatik test var.

**Çıkış kapısı:** Temiz ortamda gerçek kullanıcıyla giriş ve RBAC kanıtlanabiliyor.

### A2 — T-1 + F-1: Tarifeli prim motoru, teklif (hedef 4 saat)
- [x] Zaman geçerlilikli tarife/faktör tabloları migration ile hazır.
- [x] Prim hesaplama deterministik ve kalem dökümü veriyor.
- [x] Teklif kaydediliyor, yeniden sorgulanıyor; poliçeye dönüştürme başlangıç akışı var.
- [x] 29 Şubat / oran/yuvarlama/faktör testleri mevcut.
- [x] Bir uçtan uca API doğrulama senaryosu mevcut.

**Çıkış kapısı:** Aynı teklifin aynı tarife sürümüyle iki hesabı eşit, kalem toplamı nihai prime eşit.

### A3 — T-2 + F-2: Poliçe sürümleri, zeyil ve iptal (hedef 3 saat)
- [x] Başlangıç/bitiş tarihleri ve sürüm geçmişi korunuyor.
- [x] Pro-rata iade, kısa dönem tarife örneği ve artık yıl sınır testi var.
- [x] Belirli tarihte geçerli teminat ve poliçe sürümü sorgulanıyor.
- [x] İptal sonrası durum ve kayıt geçmişi tutarlı; audit olayı oluşuyor.

**Çıkış kapısı:** Örnek tarih aralığı için beklenen teminat/sürüm ve kuruş tutarları otomatik testle kanıtlanıyor.

### A4 — T-3 + F-4/F-5/F-6: Hasar ve tazminat (hedef 4 saat)
- [x] Hasar bildirimi, ekspertiz kaydı, tazminat hesabı ve ödeme kayıt akışı var.
- [x] Hasar tarihindeki poliçe versiyonu kullanılıyor.
- [x] Muafiyet, teminat başına ve yıllık toplam limit doğru uygulanıyor.
- [x] Eşzamanlı onay limiti aşmıyor (`FOR UPDATE` veya eşdeğeri).
- [x] Gerçek iki paralel transaction/istekle yarış durumu testi var.

**Çıkış kapısı:** İki eşzamanlı hasarda harcanan limit yıllık tavanı geçmiyor; açıklamalı ödeme kalemleri görünüyor.

### A5 — T-4 + F-3: Taksit, tahsilat, muhasebe (hedef 4 saat)
- [x] Taksit planı ve kısmi ödemenin en eski taksite tahsisi çalışıyor.
- [x] Gecikme faizi, iade, 30-gün iptal kuralı test ediliyor.
- [x] `Idempotency-Key` tekrar isteği ikinci tahsilat yaratmıyor.
- [x] Muhasebe borç/alacak kayıtları her işlemin sonunda dengede.
- [x] Ödeme/API ve iade için uçtan uca kanıt var.

**Çıkış kapısı:** Tekrarlı ödeme mali kayıt üretmiyor; defter bakiye testi başarılı.

### A6 — T-5 + F-7: Fraud iş akışı ve raporlar (hedef 3 saat)
- [x] 3 şüphe kuralı ve açıklanabilir fraud skoru tanımlı.
- [x] Şüpheli dosya inceleme kuyruğuna düşüyor; geçişler doğrulanıyor.
- [x] İş yükü en az uzmana atama ve SLA takibi deneniyor.
- [x] Prim üretimi / hasar-prim oranı SQL/API üzerinden hesaplanıyor.

**Çıkış kapısı:** Kasıtlı şüpheli seed senaryosu doğru skoru/kuyruğu tetikliyor; rapor toplamı veriyle uyuşuyor.

### A7 — Gerçek veriyle 6 rolün tamamlanması ve simülatör (hedef 3 saat)
- [ ] Her rol için kendi işiyle ilişkili kullanılabilir ekranlar var.
- [ ] Tüm ekranlar gerçek API'den veri çekiyor; loading/empty/error/mobile durumları gözden geçirildi.
- [ ] Simülatör ayrı compose servisinde, aynı seed ile aynı sonuç; hedef 100.000 poliçe + 5 yıl geçmiş.
- [ ] UI üzerinde veri akışı/canlılık gösteriliyor.
- [ ] Demo ve büyük simülasyon verisi birbirini bozmayacak şekilde ayrıldı.

**Çıkış kapısı:** Uygun yetkiyle giren her demo rolü kendi uçtan uca işini deneyebiliyor. Simülatör hedefi tamamlanamadıysa açıkça işaretle.

### A8 — Doğrulamayı tamamlama, CI, dokümantasyon, yayın (en az 4–6 saat AYIR)
- [ ] Gerçek API çağrılarıyla `make verify` F-1…F-7, T-1…T-5, health ve güvenlik kontrollerini raporluyor.
- [ ] `make reset-demo` tekrar denenebilir; `requests.http` bütün uçları içeriyor.
- [ ] 40 test / domain ≥%70 kapsam raporu gerçek CI çıktılarına dayanıyor.
- [ ] Actions: test, lint, build, container image build.
- [ ] OpenAPI ve Prometheus `/metrics` mevcut.
- [ ] k6 ile p50/p95/p99/hata oranı **ölçülmüş**; README'ye aktarılmış.
- [ ] Kaos betiği denenmiş; iki indeks için `EXPLAIN ANALYZE` karşılaştırması yazılmış.
- [ ] `DECISIONS.md` en az 5 karar; `TESTING.md` her ID ve demo hesaplarını içeriyor.
- [ ] README'nin 5 doğrulama sorusu dürüst ve somut şekilde yanıtlandı.
- [ ] Public GitHub'da **birden çok anlamlı commit** var; temiz ortamda compose çalışıyor.
- [ ] Repo bağlantısı **26 Ekim 23.59'dan önce** ankete yüklenmiş; public erişim kontrol edildi.

**Çıkış kapısı:** `make verify` gerçek çıktı + repo URL + başvuru formu teslimi. Bilinen eksikler README'de açık.

## Zaman daralınca karar kuralları

1. **Önce A1–A2** ile çalışır bir dikey akış kur. Süre kısalırsa ana çekirdek görevleri küçült, ama matematiksel olarak doğru testli örnek sun.
2. **A3–A6** içinde her çekirdeğin en az bir doğrulanabilir senaryosu önemli; yarım özelliği tamamlandı diye gösterme.
3. **A7 tasarım cilası** ve bonuslar kesilebilir; gerçek veriyle çalışan, sade ekranlar öncelikli.
4. A8 için **son 4–6 saatlik güvenlik payını koru**. CI, temiz kurulum, README, `make verify`, GitHub ve anket asla sona rastgele bırakılmamalı.
5. Kalan zaman yetmezse `REQUIREMENTS.md` içindeki zorunlulukları silme; `TESTING.md`/README'de `PASS / FAIL / YAPILMADI` ile raporla.

## Kanıt takip matrisi (yalnızca gerçekten test edilirse işaretle)

| Kimlik | Durum | Kanıt (test veya komut) |
|---|---|---|
| F-1 | PASS | `make verify` (F-1 teklif-kaydet-oku, police-cevir, cift-cevirme, uw-onay); `TestConvertCreatesPolicyAndIsIdempotent` |
| F-2 | PASS | `make verify` (F-2 zeyil-kayit, surum-gecmisi, iptal-sonrasi); `TestVehicleChangeEndorsementAndCoverageOnDate`, `TestShortPeriodCancellationAfterEndorsement` |
| F-3 | PASS | `make verify` (taksit/tahsilat/idem/aşırı ödeme/iade/gün sonu); 52 entegrasyon testi, `TESTING.md` A5 |
| F-4 | PASS | `make verify` F-4 hasar-dosya; `TestClaimVersionSelectionAndCancellation`, `TestClaimTransitionsValidationAndRBAC` |
| F-5 | PASS | `make verify` F-5 ekspertiz; `TestClaimLifecycleAndIdempotency` |
| F-6 | PASS | `make verify` F-6 idempotent-onay/odeme; `TestClaimLifecycleAndIdempotency` |
| F-7 | PASS | `TestProductionReportGoldenLedgerPeriodsAndGroups`, dönem/RBAC testi; `make verify` prim-delta, defter-mutabakati, boyut-toplami, oran |
| T-1 | PASS | `TestGoldenKasko`, `TestGoldenKonut`, `TestBreakdownSumEqualsTotal`, `TestSelectTariffByStartDate`; `make verify` T-1 satırları |
| T-2 | PASS | `TestGoldenVehicleChange`, `TestGoldenShortPeriodCancellation`, `TestCancellationOnLeapDay`; `make verify` T-2 satırları; eşzamanlılık `TestConcurrentEndorsements` |
| T-3 | PASS | `TestGoldenSettlements`; `TestConcurrentClaimApprovals -count=10 -race`; mutasyon LIMIT_CHANGED; `make verify` hesap-adimlari/paralel-limit |
| T-4 | PASS | Kayıt bazında awk faiz hesabı, gerçek paralel ödeme, 1200 mutabakatı; 10 tekrar `-race` ve kilit mutasyonu; billing %92,1 |
| T-5 | PASS | Fraud/workflow %100; kalıcı sinyaller, görev/SLA/RBAC, 20 transaction 7/7/6 ×10 `-race`, advisory mutasyonu FAIL ve geri yükleme; `make verify` yedi T-5 satırı |

## Teslim kontrolü

- [ ] Aday adı ve kurulum bilgileri README başında.
- [ ] Ajan çıktıları manuel olarak incelendi, kritik kararlar açıklanabiliyor.
- [ ] `.env`/secret repoda yok, Git remote ve branch doğru.
- [ ] Her public commit anlamlı; force-push yapılmadı.
- [ ] README ve `TESTING.md` yapılmayanları saklamıyor.
- [ ] **Anket bağlantısı gönderildi ve teslim teyit edildi.**
