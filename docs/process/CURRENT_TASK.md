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
