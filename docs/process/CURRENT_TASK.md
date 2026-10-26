# B2 — Poliçe PDF çıktısı

**Durum: ONAYLI.** B2-P0 salt okunur keşif planı kullanıcı tarafından onaylandı.
**Taban:** `ec70df6`; ayrı yerel dal `bonus/police-pdf`. Main'e merge/push yok.

## Amaç ve kapsam

Mevcut poliçe detayı yetkisiyle, son kayıtlı sürümü açıkça etiketleyen demo poliçe PDF özeti üretmek ve arayüzden indirmek.
- `GET /api/v1/policies/{id}/pdf`: mevcut okuyucu rolleri; kapsam dışına 404, eksper 403, kimliksize 401.
- Tek salt okunur repeatable-read snapshot; aynı transaction'da yetki ve veri okumaları; çizim öncesi bağlantıyı serbest bırak.
- Taksit ve yeni TCKN bölümü hasar uzmanına verilmez; diğer belge kullanıcılarında TCKN yalnız mevcut maskeyle.
- Kayıtlı prim/teminat/limit/muafiyet ve varsa özgün faktör satırları. SIM/seed için eksik faktör kaydı açıkça belirtilir; yeniden hesaplanmaz.
- GoPDF v0.38.3, statik NotoSans-Regular.ttf ve OFL 1.1; font binary'ye gömülür. Para int64 kuruş, Türkçe TL gösterimi.
- Mevcut frontend istemcisine Authorization/refresh davranışını koruyan blob indirme; mevcut detayda PDF düğmesi.
- OpenAPI, requests, README, TESTING, ADR-28, PROGRESS/ROADMAP ve tek kısa kabul kaydı.

## Korunan sınırlar ve durma koşulları

Migration, CI, Dockerfile, simülatör, k6 eşikleri ve geçmiş veri değişikliği yok; reset/down/prune/volume işlemi yok.
Yeni PDF sorgusu okuma yapar; mevcut API ve iş akışları değişmez. Import döngüsü ve açık transaction içinde ikinci pool bağlantısı kurulmaz.
100k kabulü önce/sonra salt okunur karşılaştırılır; iş alanları/parmak izi aynı, SLA yalnız artabilir. Uyumsuzlukta dur.
Kapsam dışı zorunlu sorun veya güvenli olmayan veri/yetki genişlemesinde minimal çözümle onay bekle.
Teslim 23:59 TRT; kapılar tamamlanmadan main'e alınmaz. Yerel commit serbest, push kullanıcıya ait.

## Sıra ve kabul

Backend + anlamlı birim/entegrasyon testleri → frontend → belgeler → `make test lint build` → `make test-integration` → yalnız API/web yeniden derleme → `make verify`.
Hedefli PDF testleri, rol/kapsam, para uç değerleri, Türkçe/çok sayfa görsel kabulü; gerçek çalıştırılmayan kontrol PASS sayılmaz.

## Rapor

[AGENTS.md §6](AGENTS.md#6-aşama-sonunda-zorunlu-rapor-biçimi): durum, dosyalar, test/süre/commit, PDF kabulü, 100k karşılaştırması, sınırlar ve push yapılmadığı.

## Ara kontrol noktası — kullanıcı yeni sohbete geçmek istedi

**KISMEN; kullanıcının açık isteğiyle burada duruldu.** Görev onaylıdır; yeni sohbette bu kayıttan devam edilir. Main'e merge/push yok.

### Tamamlanan kaynaklar ve commitler

- `6ef011f docs: B2 görev tanımı`
- `9908260 feat(policy): yetkili poliçe PDF çıktısını ekle`
- `9085640 feat(ui): poliçe PDF indirme işlemini ekle`
- Backend: aynı read-only repeatable-read tx'de yetki/veri; son kayıtlı sürüm; claim adjuster için TCKN/taksit yok; saklanmış quote/zeyil kalemleri, eksikse açık not; GoPDF/font/lisans; int64 TL; JSON hata/PDF attachment.
- Frontend: apiBlob mevcut send/refresh'i kullanır; detayda indirme, bekleme ve hata. Tarayıcıdan henüz denenmedi.

### Gerçek doğrulamalar

- `go test ./internal/policypdf ./internal/policy ./internal/customer`: PASS (customer test dosyası yok).
- Son `go test ./internal/policypdf ./internal/policy`: PASS; policypdf 0,699 sn, policy cached.
- `./scripts/test-integration.sh -run TestPolicyPDF -count=1`: PASS; app 7,715 sn. Log `/tmp/guvenpolice-b2/targeted.log`.
- İlk hedefli koşu FAIL/app 7,134 sn: foreign test fixture AnnualLimitKurus=0, pozitif DB kısıtı. Yalnız fixture 10000000 yapıldı; sonraki koşu PASS. Bu FAIL raporda saklanacak.
- `CGO_ENABLED=0 go build -o /tmp/guvenpolice-b2/api-check ./cmd/api`: PASS; host binary /tmp altında, Git'e alınmaz.
- `npm --prefix frontend run lint && npm --prefix frontend run build`: PASS; tsc/Vite, 49 modül, JS index-7TVXWxAB.js.
- Tam `make test lint build`, tam integration, yeni verify ve görsel PDF kabulü HENÜZ ÇALIŞTIRILMADI.

### Korunan veri ve çalışma alanı

- Dal `bonus/police-pdf`; main `ec70df60b0c5710e0e3c35b045efa13aee63b779` değişmedi. Docker servisleri B2 kaynaklarıyla henüz yeniden derlenmedi.
- Önceki 100k kabulü `/tmp/guvenpolice-b2/acceptance-before.json`: iki elemanlı JSON dizi; [0] mevcut a8-acceptance.sql sonucu, [1] ekspertiz adedi/MD5. 100000 poliçe, 7780 hasar, 6727 ekspertiz, 495 işaretli, 46 açık inceleme/görev, anlık SLA 46.
- İlk kabul yardımcı ayrıştırması çok satırlı JSON'u satır bazında okumada hata verdi; aynı salt okunur sorgu JSONDecoder.raw_decode ile başarıyla kaydedildi. DB değişikliği yapılmadı.
- Ek ekspertiz MD5 tarifi: SIM poliçelerine bağlı appraisals için md5(string_agg(a.id::text || ':' || a.claim_id::text || ':' || a.appraiser_id::text || ':' || a.damage_amount_kurus::text,',' ORDER BY a.id)).
- Son boş disk 4,953 GiB. Reset/down/prune/volume/simülasyon/migration/CI değişikliği yapılmadı.
- Noto dosyası 569208 bayt; SHA256 b85c38ecea8a7cfb39c24e395a4007474fa5a4fc864f6ee33309eb4948d232d5. Font ve OFL.txt kaynakta; tam lisans PDF Subject metadata'sına da yazılır.

### Kalan sıra — yeni sohbette

1. Bu kaydı ve son git status/log'u oku; kapsamı koru. Kod için gerekirse yalnız küçük düzeltmeler.
2. scripts/verify.sh'e tek `B2 police-pdf` satırı ekle: mevcut acente poliçesine GET, PDF/header/no-store, kimliksiz401 ve eksper403. Eski verify kimlik/satır numaralarını bozmamak için bloğu son sonuçtan önce eklemek uygun.
3. openapi.yaml (GET /api/v1/policies/{id}/pdf), requests.http, README bonus/izlenebilirlik ve yeni test sayısı, TESTING, ADR-28, PROGRESS/ROADMAP güncelle. Kişisel 5 cevap korunur. PDF/fotoğraf YAPILMADI ifadelerinde yalnız PDF kabul sonrası kaldırılır; fotoğraf yok kalır. Tarihsel kayıtları silme.
4. Test edilen son kod hash'ini belirle; `make test lint build`, ardından `make test-integration` çalıştır ve gerçek süre/sonuçları /tmp altında kaydet.
5. Yalnız `docker compose up -d --no-deps --build api web`; DB/volume'lara dokunma. Sonra `make verify` (yeni satırla beklenen toplamı gerçek çıktı belirler).
6. Yeni PDF'yi demo poliçesinden al; Türkçe metin, tutar, uzun içerik ve çok sayfayı gerçek render/metin çıkarımıyla kontrol et. Ham PDF'de düz UTF-8 arayarak PASS verme.
7. PDF skill okundu ve mark_artifact_operation_started.mjs create/pdf sayacı B2 için bir kez başarıyla çalıştırıldı. Tekrar işaretleme gerekmez. pdftoppm/pdfinfo PATH'te; pdftotext `/Users/macair/.cache/codex-runtimes/codex-primary-runtime/dependencies/native/poppler/poppler/bin/pdftotext` altında. Ara PDF/PNG'ler /tmp/guvenpolice-b2 altında, repoya değil.
8. Tarayıcıda PDF düğmesini, hata/bekleme ve 375px'i doğrula. Yeni özellik oluşturma/ödeme yapma; yalnız mevcut demo kaydı.
9. Aynı a8 kabul sorgusunu READ ONLY ile sonra al, ek ekspertiz/MD5'i karşılaştır. İş alanları/parmak izi/özet aynı, açık görev aynı, SLA yalnız artabilir. measured_at/database_bytes farkını ayrıca yaz. Uyumsuzlukta dur.
10. Tek kısa `docs/testing/b2-pdf.md` kabul kaydı, son süreç kapanışı, temiz Git ve AGENTS §6 raporu. Yeni kanıt klasörü/log yığını oluşturma; loglar /tmp'de. Main'e merge/push kullanıcıya ait.

Teslim 26 Ekim 2026 23:59 TRT. Kaynak durumunu TAMAMLANDI veya bonusu teslim edilmiş olarak henüz işaretleme.
