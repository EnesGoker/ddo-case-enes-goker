#!/usr/bin/env bash
# Yalnız bu compose projesinin DB'sine kontrollü kesinti; her çıkışta geri açılır.
set -euo pipefail
cd "$(dirname "$0")/.."
python3 - <<'PY'
import datetime, json, os, secrets, subprocess, time, urllib.error, urllib.request
from pathlib import Path
base=os.getenv('API_URL','http://localhost:8080')
password=os.getenv('DEMO_PASSWORD')
if not password:
    password=next((s.split('=',1)[1].strip('"') for s in Path('.env').read_text().splitlines() if s.startswith('DEMO_PASSWORD=')),None)
if not password: raise SystemExit('DEMO_PASSWORD gerekli')
def compose(*args):
    return subprocess.check_output(['docker','compose',*args],stderr=subprocess.STDOUT,text=True).strip()
def request(method,path,token='',body=None,key=None):
    headers={'Content-Type':'application/json'}
    if token: headers['Authorization']='Bearer '+token
    if key: headers['Idempotency-Key']=key
    req=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers=headers,method=method)
    try:
        with urllib.request.urlopen(req,timeout=25) as r: status,raw=r.status,r.read()
    except urllib.error.HTTPError as e: status,raw=e.code,e.read()
    try: return status,json.loads(raw)
    except ValueError: return status,{}
def require(ok,message):
    print(('PASS  ' if ok else 'FAIL  ')+message,flush=True)
    if not ok: raise RuntimeError(message)
def login(name):
    s,b=request('POST','/api/v1/auth/login',body={'email':name+'@guvenpolice.test','password':password})
    require(s==200,'giriş '+name)
    return b['access_token']
def state():
    cid=compose('ps','-q','api')
    return json.loads(subprocess.check_output(['docker','inspect',cid],text=True))[0]
account,agent,uw=login('muhasebe'),login('acente'),login('underwriter')
s,b=request('GET','/api/v1/system/activity',account)
require(s==200 and (not b.get('simulation') or b['simulation']['status'] not in ('RUNNING','LIVE')),'simülatör kapalı')
s,b=request('GET','/api/v1/customers?limit=100',agent)
require(s==200,'demo müşteriyi oku')
customer=next(x['id'] for x in b['items'] if x['national_id_masked'].endswith('0001'))
key='chaos-'+str(time.time_ns())
plate='34CHS'+f'{secrets.randbelow(10000):04d}'
s,q=request('POST','/api/v1/quotes',agent,{'customer_id':customer,'product':'KASKO','term_start':datetime.date.today().isoformat(),'ncd_level':2,'vehicle_group':'C','plate':plate,'coverages':['KASKO_ANA']})
require(s==201,'ayrı kaos poliçesi teklifi')
if q['status']=='UW_REVIEW':
    s,_=request('POST',f'/api/v1/quotes/{q["id"]}/review',uw,{'decision':'APPROVE','note':'kaos kabulü'})
    require(s==200,'teklif onayı')
s,p=request('POST',f'/api/v1/quotes/{q["id"]}/convert',agent,{},key+'-policy')
require(s==201,'kaos poliçesi düzenle')
path=f'/api/v1/policies/{p["id"]}/payments'
body={'amount_kurus':1,'method':'CARD'}
before=state()
started=False
try:
    started=True
    compose('stop','db')
    s,b=request('GET','/health')
    require(s==503 and b.get('db')=='down','(a) sağlık 503 db=down')
    s,b=request('GET','/api/v1/policies?limit=1',agent)
    require(s>=500 and isinstance(b.get('error'),dict) and bool(b['error'].get('code')),'(a) iş ucu JSON 5xx')
    s,b=request('POST',path,account,body,key)
    require(s>=500 and isinstance(b.get('error'),dict),'(c) kesintide ödeme başarısız/JSON')
    down=state()
    require(down['Id']==before['Id'] and down['RestartCount']==before['RestartCount'] and down['State']['Running'],'(a) API aynı konteynerde, yeniden başlamadı')
    recovery=time.monotonic()
    compose('start','db')
    healthy=False
    while time.monotonic()-recovery<60:
        try:
            s,b=request('GET','/health')
            if s==200 and b.get('db')=='ok': healthy=True;break
        except (OSError,TimeoutError): pass
        time.sleep(.25)
    elapsed=time.monotonic()-recovery
    require(healthy,f'(b) DB iyileşti: {elapsed:.3f} saniye (<=60)')
    s,b=request('GET','/api/v1/policies?limit=1',agent)
    require(s==200 and isinstance(b.get('items'),list),'(b) iş ucu tekrar 200')
    s,first=request('POST',path,account,body,key)
    require(s==201,'(c) aynı anahtarla ödeme 201')
    s,replay=request('POST',path,account,body,key)
    require(s==201 and first==replay,'(c) tekrar aynı ödeme')
    sql=f"SELECT count(*),count(DISTINCT journal_entry_id) FROM payments WHERE request_key='{key}' AND policy_id={p['id']};"
    result=subprocess.check_output(['docker','compose','exec','-T','db','sh','-c','psql -At -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'],input=sql,text=True).strip()
    require(result=='1|1','(c) DB: tek ödeme/tek yevmiye')
    s,b=request('GET','/api/v1/ledger/trial-balance',account)
    require(s==200 and b.get('balanced') and b.get('receivable_reconciliation',{}).get('matches'),'(c) defter dengesi ve 1200 mutabakatı')
    after=state()
    require(after['Id']==before['Id'] and after['RestartCount']==before['RestartCount'],'(b) API kesinti/iyileşmede yeniden başlamadı')
    print('PASS  kaos tamamlandı',flush=True)
finally:
    if started:
        compose('up','-d','--wait','db')
PY
