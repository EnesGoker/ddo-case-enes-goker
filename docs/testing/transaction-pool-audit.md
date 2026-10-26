# Açık transaction / ek havuz bağlantısı taraması

26 Ekim A8: kullanıcı onayıyla CI'de görülen üretim hatası bütün servislere tarandı. Kanıt: [ilk CI](https://github.com/EnesGoker/ddo-case-enes-goker/actions/runs/40281907975), entegrasyon TestConcurrentEndorsements'te beş istemci10sn timeout. Diğer dört CI işi PASS.

pgxpool varsayılan MaxConns en az4, CPU sayısına göre büyür. İki CPU'lu CI/Docker ortamında4'tür. Dört eşzamanlı işlem bağlantıları transaction için tutarsa, aktif işlem access.Resolve aracılığıyla beşinci bağlantıyı isteyebilir; diğer transaction'lar onun satır kilidini bekler. Havuzu büyütmek veya timeout'u uzatmak kök nedeni çözmez.

## Düzeltilen beş bağ

| Yardımcı | Etkilenen yollar | Düzeltme |
|---|---|---|
| policy.replayChange | VehicleChange, Cancel/CancelTx; gün sonundan CancelTx dahil | access.Resolve(tx) |
| policy.ChangeService.target | zeyil/iptal planı ve iki readonly önizleme | access.Resolve(q) |
| payment.Service.target | Pay, Refund, readonly Billing | access.Resolve(q) |
| claim.Service.visible | ChangeStatus, Appraise, Approve, Pay; Get/Preview havuzla çağırmaya devam eder | access.Resolve(q) |
| access.Resolve/lookup | Yukarıdaki ortak profil sorgusu | mevcut db.DBTX; hem pool hem tx uygulanabilir |

Hasar mutasyonlarının mevcut izinli iç rollerinde Resolve zaten profil sorgusu yapmıyordu; yine de ortak yardımcı transaction yolunda havuza dönmeyecek şekilde bağlandı. API/RBAC/izolasyon kuralı, kilit sırası, para hesabı ve süre bütçesi değişmedi. Yeni migration yok;100k yeniden üretilmedi.

## Taranıp ek bağlantı bulmayan yollar

- **quote:** Calculate/scope transaction'dan önce; Create/Review sonrası Get, transaction commit'inden sonra. Convert scope önce alınır, callback sadece tx/Issue kullanır.
- **auth:** Login kullanıcı okuması bitince CreateRefresh transaction'ı; Refresh manuel Begin sonrasında token/kullanıcı okuma ve rotasyon tx ile; Logout ve repo.CreateRefresh tx ile.
- **payment/store, policy Issue/Change yardımcıları:** ledger/audit/limit/plan işlemleri verilen q/tx üzerinden. Refund kaynak poliçe sorgusu Begin'den önce. Run aday rows'u CollectRows ile kapandıktan sonra poliçe transaction'larına geçer.
- **Run → NewChangeService(pool).CancelTx(tx):** callback içindeki tek kalan `.pool` ifadesi constructor parametresidir; constructor DB erişimi yapmaz, CancelTx mevcut tx'yi taşır. Yeni yetki bağları da tx'yi kullanır.
- **claim.Report:** scope Begin öncesi. visible'nin appraiser sahiplik sorgusu q ile; tüm settlement/Transition/task/ledger/audit yardımcıları tx ile. Get/List transaction açmadan havuz kullanır.
- **task:** atama/Complete/Advance tx; liste/iş yükü/SLA HTTP okumaları açık transaction içinde değil.
- **ledger/report:** mizan readonly tx→ReadTrialBalance(tx); Entries ve Production tek sorgu/kapalı rows. İçeride ikinci havuz edinimi yok.
- **idempotency:** acquire SQL'i handler transaction'ından önce tamamlanır; save/release handler dönüşünden sonra. Store bağlantıyı açık transaction olarak tutmaz.
- **seed:** Demo callback'inde tx yardımcıları; fraudScenario Demo/ensure transaction'ı döndükten sonra servis çağırır.
- **simulation:** referans/Issue callback'leri tx; payDues/Cancel callback döndükten sonra çağrılır. Run'ın global advisory guard bağlantısı transaction değildir; workers+4 havuz bütçesinde bilinçli ayrılmıştır.
- **customer/system/metrics/tariff:** transaction açmadan tekil okuma veya çağıranın verdiği DBTX; yeni iç havuz edinimi yok. Platform migration ayrı startup bağlantısıdır.

Taramada tüm üretim Go kaynaklarında Begin/BeginFunc/BeginTxFunc, doğrudan pool.Query/Exec/Acquire ve yardımcı çağrılar izlendi. Geçici AST taraması transaction callback'i içinde yalnız yukarıdaki Run constructor ifadesini buldu; dolaylı çağrılar elle kontrol edildi. AST tarayıcı teslim uygulamasına/bağımlılıklarına eklenmedi.

## Regresyon ve gerçek sonuç

`TestTransactionScopeUsesExistingConnectionAtPoolCapacity`: dört bağlantılı ayrı havuzun BeforeAcquire bariyeriyle dört işlem aynı anda bağlantı tutar. Zeyil, iptal, araç/iptal önizlemesi, müşteri tahsilatı ve plan okuması altı alt senaryodur. Mevcut iş çakışmaları korunur; teknik deadline beklenmez.

Yeni testin ilk derlemesinde require.True dönüş değeri olmadığı için derleme FAIL; assert.True ile test düzeltildi. Üretim düzeltmesinden **önce** endorsement alt testi5sn deadline ile gerçek FAIL verdi (/tmp/gp-a8-pool-red-runtime.log). **Sonra** tüm altı senaryo PASS0,33sn; eski dört paralel mali testle birlikte app5,874sn (/tmp/gp-a8-pool-green.log). Başarısız test/üretim sürümü commit'lenmedi. Tam paket ve yeni CI sonucu PROGRESS'te ayrıca kaydedilir.

Son yerel tam doğrulama: `make test-integration` PASS/exit0, app79,391sn; `make test lint build` PASS/exit0. Tam tarama sonrasında transaction callback'inde tek kalan pool referansı Run'daki constructor'dır (DB edinimi yapmaz).
