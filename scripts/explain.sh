#!/usr/bin/env bash
# Tek başına çalıştır: indeks kaldırma ROLLBACK ile aynı transaction'da geri alınır.
set -euo pipefail
cd "$(dirname "$0")/.."
python3 - <<'PY'
import datetime, json, os, re, statistics, subprocess
from pathlib import Path
root=Path('.')
out=root/'docs/explain';out.mkdir(parents=True,exist_ok=True)
def sql(query):
    return subprocess.check_output(['docker','compose','exec','-T','db','sh','-c',
      'psql -XAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'],input=query,text=True)
status=sql("SELECT count(*) FROM simulation_runs WHERE status IN ('RUNNING','LIVE');").strip()
if status!='0': raise SystemExit('Simülatör kapanmadan EXPLAIN çalışmaz')
# Planner istatistiği iki varyant için aynıdır; plan zorlayıcı ayar yok.
sql('ANALYZE payments; ANALYZE claims; ANALYZE journal_entries; ANALYZE journal_lines;')
pid=int(sql("SELECT p.policy_id FROM payments p JOIN policies po ON po.id=p.policy_id WHERE po.policy_no LIKE 'SIM-%' ORDER BY p.id LIMIT 1;").strip())
repo=(root/'backend/internal/payment/repo.go').read_text()
query=re.search(r'(SELECT p.id, p.policy_id, p.kind.*?ORDER BY p.paid_at DESC, p.id DESC)',repo,re.S).group(1).replace('$1',str(pid))
report=(root/'backend/internal/report/report.go').read_text()
reportquery=re.search(r'q.Query\(ctx, `(.*?)`, filter.From',report,re.S).group(1)
anchor=sql("SELECT (parameters->>'anchor')::date::text FROM simulation_runs WHERE status='COMPLETED' ORDER BY id DESC LIMIT 1;").strip()
# parameters sürümleri arasında anahtar değişmişse son iş tarihini kullan.
if not re.fullmatch(r'\d{4}-\d{2}-\d{2}',anchor): anchor=sql("SELECT max(loss_date)::text FROM claims;").strip()
to=datetime.date.fromisoformat(anchor)+datetime.timedelta(days=1);start=to-datetime.timedelta(days=30)
reportquery=reportquery.replace('$1',f"'{start}'::date").replace('$2',f"'{to}'::date").replace('$3',"'product'::text")
workshop,cid,loss=sql("SELECT c.workshop_id,c.id,c.loss_date FROM claims c JOIN policies p ON p.id=c.policy_id WHERE p.policy_no LIKE 'SIM-%' AND c.workshop_id IS NOT NULL ORDER BY c.loss_date DESC,c.id DESC LIMIT 1;").strip().split('|')
fromdate=datetime.date.fromisoformat(loss)-datetime.timedelta(days=29)
workquery=f"SELECT claim_no FROM claims WHERE workshop_id={int(workshop)} AND id<>{int(cid)} AND loss_date BETWEEN '{fromdate}'::date AND '{loss}'::date ORDER BY loss_date,id"
queries=[('refund','payments_refund_of_payment_idx',query),('report','journal_entries_date_idx',reportquery),('workshop','claims_workshop_loss_idx',workquery)]
rows=[]
for name,index,query in queries:
    (out/f'{name}.sql').write_text(query.strip()+';\n')
    measurements={}
    for indexed in (True,False):
        variant='indexed' if indexed else 'without-index'
        prefix="BEGIN; SET LOCAL lock_timeout='5s'; SET LOCAL statement_timeout='120s';\n"
        if not indexed: prefix+=f'DROP INDEX {index};\n'
        # psql hata verirse bağlantı kapanır ve transaction kendiliğinden rollback olur.
        text=sql(prefix+'EXPLAIN (ANALYZE, BUFFERS) '+query+';\nROLLBACK;')
        (out/f'{name}-{variant}.txt').write_text(text)
        runs=[]
        for _ in range(3):
            raw=sql(prefix+'EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) '+query+';\nROLLBACK;')
            # psql transaction komut etiketlerini ayıkla; JSON planı değiştirme.
            begin=raw.index('[\n');end=raw.rindex(']')+1
            plan=json.loads(raw[begin:end])[0];runs.append(plan)
        (out/f'{name}-{variant}.json').write_text(json.dumps(runs,indent=2)+'\n')
        mid=sorted(runs,key=lambda p:p['Execution Time'])[1]
        node=mid['Plan']
        measurements[variant]={'execution_ms_median':statistics.median(p['Execution Time'] for p in runs),
          'shared_hit_blocks':node.get('Shared Hit Blocks',0),'shared_read_blocks':node.get('Shared Read Blocks',0),
          'actual_rows':node.get('Actual Rows'), 'node':node.get('Node Type')}
        print(name,variant,json.dumps(measurements[variant]),flush=True)
    valid=sql(f"SELECT indisvalid AND indisready FROM pg_index WHERE indexrelid='{index}'::regclass;").strip()
    if valid!='t':raise RuntimeError('ROLLBACK sonrası indeks geçerli değil: '+index)
    rows.append({'query':name,'index':index,'variants':measurements})
(out/'summary.json').write_text(json.dumps({'measured_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'policy_id':pid,'from':str(start),'to_exclusive':str(to),'rows':rows},indent=2)+'\n')
print('PASS tüm indeksler ROLLBACK sonrası geçerli/hazır')
PY
