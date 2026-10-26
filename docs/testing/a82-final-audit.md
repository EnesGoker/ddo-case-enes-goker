# A8.2 son belge kontrolleri

Başlangıç: 4e71d8c; git durumu temizdi. Yalnız belgeler düzenlendi. Kod/test/migration/CI/betik değiştirilmedi; Docker, test, verify, simülasyon, paket kurulumu ve push çalıştırılmadı.

## 1. İlk üç kontrol

**1.1 Zaman karşılaştırması:** Karşılaştırma kaldırılmadı. Yalnız inspected_at, RFC3339Nano ayrıştırmasıyla saat diliminden bağımsız an olarak ve PostgreSQL'in mikrosaniye hassasiyetinde karşılaştırılır. Ardından bu alan normalize edilerek kaydın bütün diğer alanları birebir karşılaştırılır. HTTP201, ekspertiz=1, CLAIM_APPRAISED=1 ve financialCounts eşitliği korunur. Bayat yanıt senaryosuna nanosaniyeli girdi eklenmiştir; bu ek kapsam karşılaştırmayı gevşetmez.

FAIL'deki ara test sürümü ayrı commit değildi. Aşağıdaki diff önceki oturumdaki düzenleme ve saklanan a81-appraisal-after.log'daki assert.Equal başarısızlığından yeniden oluşturulmuştur; tarihsel bir git diff çıktısı olarak sunulmaz. Gösterim, yalnız değişen satırları içeren sıfır bağlamlı diff'tir. O kayıtta ID/created_at/tutar aynı, inspected_at Z/+03:00 gösterimi farklıdır. Mevcut kanıt: backend/internal/app/appraisal_recovery_integration_test.go:38,65,72,125 ve docs/testing/a81-appraisal-after.log:6.

```diff
--- FAIL-sonrası-ara-sürüm-yeniden-oluşturma
+++ mevcut-test
@@ -37,0 +38,4 @@
+			inspected := time.Now().UTC().Truncate(time.Second)
+			if recovery == "stale_response" {
+				inspected = inspected.Add(123456789 * time.Nanosecond)
+			}
@@ -40 +44 @@
-				"inspected_at":        time.Now().UTC().Format(time.RFC3339),
+				"inspected_at":        inspected.Format(time.RFC3339Nano),
@@ -61 +65 @@
-			assert.Equal(t, first, replay, "kurtarma aynı ekspertizi döndürmeli")
+			assertSameAppraisal(t, first, replay)
@@ -109 +113 @@
-	assert.Equal(t, first, recovered)
+	assertSameAppraisal(t, first, recovered)
@@ -118,0 +123,16 @@
+}
+
+func assertSameAppraisal(t *testing.T, first, replay map[string]any) {
+	t.Helper()
+	firstTime, err := time.Parse(time.RFC3339Nano, first["inspected_at"].(string))
+	require.NoError(t, err)
+	replayTime, err := time.Parse(time.RFC3339Nano, replay["inspected_at"].(string))
+	require.NoError(t, err)
+	assert.True(t, firstTime.Truncate(time.Microsecond).Equal(replayTime.Truncate(time.Microsecond)),
+		"kurtarma aynı inceleme anını PostgreSQL hassasiyetinde korumalı")
+	canonical := make(map[string]any, len(replay))
+	for field, value := range replay {
+		canonical[field] = value
+	}
+	canonical["inspected_at"] = first["inspected_at"]
+	assert.Equal(t, first, canonical, "kurtarma aynı ekspertiz kaydını döndürmeli")
```

**1.2 ADR-14:** DECISIONS.md:148 ve 152–160 doğru ayrımı yapıyor: altı iş anahtarlı uç aynı kayıt/201 ile kurtarılır; teklif dönüştürme, hasar onayı ve hasar ödemesinde durum/kısıt ikinci etkiyi engeller ve kayıp yanıt sonrasında 409 alınır. Normal middleware cache replay'i ile çökme kurtarması ayrı açıklanmıştır. Değişiklik gerekmedi.

**1.3 SLA:** README.md:166 zaten “Açık görevlerin SLA durumu gerçek saat ilerledikçe yaşlanır; oran bu ölçüm anına aittir.” diyor. Tekrar cümle eklenmedi.

## 2. Sır taraması

Başlangıç HEAD'inin 273 UTF-8 metin dosyası tarandı; binary dosyalar kapsam dışında. Token/Authorization literal değerleri, parola atamaları, özel anahtar/API anahtarı kalıpları ve yerel .env'deki hassas ayar değerleriyle eşleşmeler kontrol edildi. Ham aday değerleri çıktıya veya bu kayda yazılmadı.

Gerçek görünümlü kullanılabilir sır/JWT bulunmadı; `<redacted>` uygulanan dosya yok. Özellikle docs/testing kayıtlarında sır adayı bulunmadı. Yerel .env'nin örnek dosyadaki demo ayarlarıyla eşleşmesi aşağıda açıkça sınıflandırıldı; .env ve örnek yapılandırma değiştirilmedi. README'nin değerlendirici için bilerek verilen demo bilgileri korundu.

| Dosya:satır | Eşleşme ve karar |
|---|---|
| .env.example:6 | Demo PostgreSQL ayarı; örnek/değiştirilebilir yapılandırma |
| .env.example:10 | Demo JWT ayarı; örnek/değiştirilebilir yapılandırma |
| .env.example:18 | Demo giriş parolası; örnek/değiştirilebilir yapılandırma |
| README.md:35 | Kullanıcının açıkça korunmasını istediği demo giriş bilgisi |
| TESTING.md:7 | Değerlendirici demo giriş bilgisi |
| backend/internal/app/integration_test.go:42 | Entegrasyon testinin demoPassword sabiti; gerçek uygulama anahtarı değil |
| requests.http:4 | Bilinçli demo parola değişkeni |
| requests.http:17 | Parola değişkenine başvuru; literal sır değil |
| requests.http:81 | Parola değişkenine başvuru; literal sır değil |
| requests.http:157 | Parola değişkenine başvuru; literal sır değil |
| requests.http:164 | Parola değişkenine başvuru; literal sır değil |
| requests.http:171 | Parola değişkenine başvuru; literal sır değil |
| scripts/verify.sh:107 | Bilerek bozuk, boş claims/yanlış imzalı negatif JWT testi; kullanılabilir token değil |

`GIT_OPTIONAL_LOCKS=0 git log -p --all --no-ext-diff --no-color` akışında 109 commit / 55.810 diff satırı yalnız özel anahtar ve gerçek API anahtarı kalıpları için tarandı; eşleşme yok. Git geçmişi değiştirilmedi. HEAD ve geçmiş sonuçları kalıp taramasının kapsamına aittir; test/demo sabitleri gizlice sır olarak silinmedi.

## 3. Okunabilirlik cümlesi

REQUIREMENTS.md'de genel okunabilirlik turu, zorunluluk veya ayrı değerlendirme ölçütü olarak geçmiyor. İlgili kelime araması eşleşme vermedi; teslim/puan ölçütleri REQUIREMENTS.md:118–127'de. README bonus satırındaki yalnız istenen iki cümle çıkarıldı; “Bonuslar: ekspertiz notu mevcut; acente performans panosu, poliçe PDF'i ve fotoğraf yükleme **YAPILMADI**.” aynen korundu.

## 4. a–j teknik doğrulaması

Aşağıdaki doğrulamalar mevcut kaynak/kayıt okumalarıdır; bu görevde test veya ölçüm yeniden çalıştırılmadı.

| İddia | Sonuç ve kapsam | Kanıt |
|---|---|---|
| a | Doğrulandı: poliçe/ödeme/hasar iş verisi ortak servislerden; referans ve koşu metadata SQL'i ADR-19 istisnası. | [DECISIONS.md:202](../../DECISIONS.md#L202), [backend/internal/simulation/policies.go:211](../../backend/internal/simulation/policies.go#L211), [backend/internal/simulation/claims.go:126](../../backend/internal/simulation/claims.go#L126), [backend/internal/simulation/claims.go:152](../../backend/internal/simulation/claims.go#L152) |
| b | Doğrulandı: son kabul edilen v4/4155bae veri kümesi tek batch, 1826,409721 sn (~30 dk); makine kaydı 8 GB. Eski üretici koşuları tarihsel kayıtlarda ayrıca vardır; “tek sefer” son kabul veri kümesi bağlamındadır. | [PROGRESS.md:817](../../PROGRESS.md#L817), [PROGRESS.md:829](../../PROGRESS.md#L829), [PROGRESS.md:831](../../PROGRESS.md#L831), [docs/testing/a8-100k-acceptance.json:7](a8-100k-acceptance.json#L7) |
| c | Doğrulandı: aynı seed, anchor ve iş parametrelerinde aynı parmak izi; worker sayısı sonucu değiştirmiyor. Seed tek başına farklı anchor/parametrelerden bağımsızlık iddiası değildir. | [backend/internal/app/simulation_integration_test.go:22](../../backend/internal/app/simulation_integration_test.go#L22), [backend/internal/app/simulation_integration_test.go:157](../../backend/internal/app/simulation_integration_test.go#L157), [PROGRESS.md:793](../../PROGRESS.md#L793), [DECISIONS.md:210](../../DECISIONS.md#L210) |
| d | Doğrulandı: sim Compose profili ve ayrı SIM veri/aktör alanı. Fiziksel DB ayrılığı iddiası yok; görev atama havuzu onaylı ortak istisna. | [docker-compose.yml:63](../../docker-compose.yml#L63), [DECISIONS.md:216](../../DECISIONS.md#L216) |
| e | Doğrulandı: 7780 hasar; balanced ve receivable_matches true. | [docs/testing/a8-100k-acceptance.json:68](a8-100k-acceptance.json#L68), [docs/testing/a8-100k-acceptance.json:133](a8-100k-acceptance.json#L133), [docs/testing/a8-100k-acceptance.json:137](a8-100k-acceptance.json#L137) |
| f | Doğrulandı: atölye 30 < eşik 60; EARLY 40 ile 70, REPEAT 35 ile 65. | [backend/internal/fraud/fraud.go:25](../../backend/internal/fraud/fraud.go#L25), [docs/testing/a8-100k-acceptance.json:13](a8-100k-acceptance.json#L13) |
| g | Doğrulandı: 300 atölye; eşik 60; 495 / 7780 = %6,36. | [backend/internal/simulation/references.go:121](../../backend/internal/simulation/references.go#L121), [docs/testing/a8-100k-acceptance.json:13](a8-100k-acceptance.json#L13), [docs/testing/a8-100k-acceptance.json:137](a8-100k-acceptance.json#L137), [PROGRESS.md:829](../../PROGRESS.md#L829) |
| h | Doğrulandı: mutlak atölye eşiği ve normal hacme göre olmaması bilinen sınırlamalarda. | [README.md:181](../../README.md#L181), [DECISIONS.md:180](../../DECISIONS.md#L180) |
| i | Doğrulandı: 16c7a6f eski billing.Adjust(effective) yolunu pozitif ek primde max(etki,bugün) ile değiştirmiş. Ödenmiş/geçmiş etkili poliçenin ACTIVE kaldığını sınayan regresyon mevcut; ikinci model hakkındaki kişisel gözlem doğrulanmadı. | [backend/internal/payment/store/adjustment.go:38](../../backend/internal/payment/store/adjustment.go#L38), [backend/internal/app/corrections_integration_test.go:26](../../backend/internal/app/corrections_integration_test.go#L26), [DECISIONS.md:166](../../DECISIONS.md#L166) |
| j | Doğrulandı: ilk CI entegrasyon havuz kilitlenmesi kaydı; ec8fef8 mevcut DBTX bağlantısına geçiriyor ve beş bağı/tüm servis taramasını belgeliyor. Yerel testte görünmemesi kişisel gözlem olarak doğrulanmadı. | [PROGRESS.md:835](../../PROGRESS.md#L835), [docs/testing/transaction-pool-audit.md:3](transaction-pool-audit.md#L3), [docs/testing/transaction-pool-audit.md:11](transaction-pool-audit.md#L11) |

Eski %24 oranının yeri yalnız raporlandı: TESTING.md:365 ve DECISIONS.md:206. Oranın nedenini Enes'in veritabanında bulması, ikinci modelin hatayı yakalaması ve yerel testte görünmemesi kişisel ifadeleri ayrıca doğrulanmaya çalışılmadı.

## 5. Metin ve biçim koruması

Enes'in 1/4 cevapları yalnız ordered-list altına üç boşluk girintiyle kondu. Yeni cümle/kelime eklenmedi veya çıkarılmadı; iç paragraf/bullet/bold/backtick düzeni korundu. Ana liste 1–5; mevcut 2/3/5 cevapları değişmedi. Girinti kaldırılınca yazar metni SHA256:

- Cevap 1: eb7b2685858b34f3650a577759540a42335e7aad27c5124f9bf62efcf31f57b3
- Cevap 4: 784539902e0b15a12a2df42c4d61eaf2e8594cd36d468c9805408437394ee322

İlk yeni belge whitespace kontrolü, bağlamlı diff gösterimindeki boşluk/sekme ve boş satır öneklerine uyarı verdi; belge içindeki gösterim sıfır bağlamlı diff olarak düzeltildi. Kaynak teste dokunulmadı. Son belge kontrolleri PASS: iki yazar metninin SHA256 eşitliği, elle girinti incelemesi, 1–5 sırası, mevcut 2/3/5 ve bonus cümlesinin korunması, 31 yerel kanıt bağlantısının dosya/satır kontrolü, yalnız izinli Markdown dosyalarının değişmesi ve `git diff --check`. Değişen belgelerde JWT/özel anahtar/API anahtarı kalıpları bulunmadı. Yeni uygulama testi/CI/verify sonucu iddia edilmedi; kullanıcı talimatıyla çalıştırılmadılar. Push yok.
