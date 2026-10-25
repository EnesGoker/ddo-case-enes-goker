#!/usr/bin/env bash
# Ayakta ve seed edilmiş sisteme karşı gerçek HTTP kontrolleri. Yalnızca curl + POSIX araçları.
# Her kontrol PASS/FAIL yazar; herhangi bir FAIL'de çıkış kodu 1'dir.
#   API_URL (varsayılan http://localhost:8080), WEB_URL (varsayılan http://localhost:3000),
#   DEMO_PASSWORD (yoksa .env'den okunur)
set -u

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
API_URL="${API_URL:-http://localhost:8080}"
WEB_URL="${WEB_URL:-http://localhost:3000}"

if [ -z "${DEMO_PASSWORD:-}" ] && [ -f "$ROOT/.env" ]; then
  DEMO_PASSWORD="$(sed -n 's/^DEMO_PASSWORD=//p' "$ROOT/.env" | tail -n 1 | sed 's/^"\(.*\)"$/\1/')"
fi
if [ -z "${DEMO_PASSWORD:-}" ]; then
  echo "DEMO_PASSWORD bulunamadı (.env veya ortam değişkeni)" >&2
  exit 2
fi

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
PASSED=0
FAILED=0
STATUS=""

pass() { PASSED=$((PASSED + 1)); printf '  PASS  %-26s  %s\n' "$1" "$2"; }
fail() { FAILED=$((FAILED + 1)); printf '  FAIL  %-26s  %s\n' "$1" "$2"; }

# req METHOD URL [curl argümanları...]  -> STATUS ve $TMP/body
req() {
  local method="$1" url="$2"
  shift 2
  STATUS="$(curl -sS -o "$TMP/body" -w '%{http_code}' --max-time 10 -X "$method" "$@" "$url" 2>"$TMP/err")" || STATUS="000"
}

# JSON gövdeden düz metin alan değeri (API tek satır JSON döner)
field() { sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" "$TMP/body" | head -n 1; }
body_has() { grep -q -- "$1" "$TMP/body"; }

login() { # login EMAIL PAROLA [JAR]
  local jar="${3:-$TMP/jar-tmp}"
  req POST "$API_URL/api/v1/auth/login" -c "$jar" -H 'Content-Type: application/json' \
    --data "{\"email\":\"$1\",\"password\":\"$2\"}"
}

echo "GüvenPoliçe verify — API: $API_URL  WEB: $WEB_URL"
echo

# --- Sağlık ve gözlem ---
req GET "$API_URL/health"
if [ "$STATUS" = 200 ] && body_has '"db":"ok"'; then pass HEALTH "/health 200, db=ok"
else fail HEALTH "/health durum=$STATUS"; fi

req GET "$API_URL/metrics"
if [ "$STATUS" = 200 ] && body_has 'http_requests_total'; then pass METRICS "/metrics Prometheus biçimi"
else fail METRICS "/metrics durum=$STATUS"; fi

req GET "$WEB_URL/"
if [ "$STATUS" = 200 ] && body_has 'GüvenPoliçe'; then pass WEB-UP "frontend :3000 200"
else fail WEB-UP "frontend durum=$STATUS"; fi

# --- 6 rolün girişi ve /me ---
for pair in musteri:CUSTOMER acente:AGENT underwriter:UNDERWRITER eksper:APPRAISER hasar:CLAIMS_ADJUSTER muhasebe:ACCOUNTING; do
  user="${pair%%:*}"
  role="${pair#*:}"
  login "$user@guvenpolice.test" "$DEMO_PASSWORD"
  token="$(field access_token)"
  if [ "$STATUS" != 200 ] || [ -z "$token" ]; then
    fail "LOGIN-$role" "giriş durum=$STATUS"
    continue
  fi
  req GET "$API_URL/api/v1/auth/me" -H "Authorization: Bearer $token"
  if [ "$STATUS" = 200 ] && [ "$(field role)" = "$role" ]; then pass "LOGIN-$role" "giriş + /me rol=$role"
  else fail "LOGIN-$role" "/me durum=$STATUS rol=$(field role)"; fi
  # sonraki RBAC kontrolleri için sakla
  eval "TOKEN_$role=\$token"
done

# --- Kimlik doğrulama hataları ---
login "acente@guvenpolice.test" "kesinlikle-yanlis-parola"
if [ "$STATUS" = 401 ] && [ "$(field code)" = INVALID_CREDENTIALS ]; then pass AUTH-WRONG-PASSWORD "401 INVALID_CREDENTIALS"
else fail AUTH-WRONG-PASSWORD "durum=$STATUS kod=$(field code)"; fi

req GET "$API_URL/api/v1/customers"
if [ "$STATUS" = 401 ] && [ "$(field code)" = UNAUTHENTICATED ]; then pass AUTH-NO-TOKEN "token'sız 401 UNAUTHENTICATED"
else fail AUTH-NO-TOKEN "durum=$STATUS kod=$(field code)"; fi

req GET "$API_URL/api/v1/customers" -H "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.e30.sahte"
if [ "$STATUS" = 401 ]; then pass AUTH-BAD-TOKEN "sahte JWT 401"
else fail AUTH-BAD-TOKEN "durum=$STATUS"; fi

# --- Rol yetkisi (sunucu tarafı) ---
for role in CUSTOMER APPRAISER CLAIMS_ADJUSTER ACCOUNTING; do
  eval "t=\${TOKEN_$role:-}"
  req GET "$API_URL/api/v1/customers" -H "Authorization: Bearer $t"
  if [ "$STATUS" = 403 ] && [ "$(field code)" = FORBIDDEN ]; then pass "RBAC-$role-403" "/customers yasak"
  else fail "RBAC-$role-403" "durum=$STATUS kod=$(field code)"; fi
done
for role in AGENT UNDERWRITER; do
  eval "t=\${TOKEN_$role:-}"
  req GET "$API_URL/api/v1/customers" -H "Authorization: Bearer $t"
  if [ "$STATUS" = 200 ] && body_has '"items":\['; then pass "RBAC-$role-200" "/customers izinli"
  else fail "RBAC-$role-200" "durum=$STATUS"; fi
done

# --- Refresh rotasyonu (cookie jar) ---
JAR="$TMP/jar"
login "underwriter@guvenpolice.test" "$DEMO_PASSWORD" "$JAR"
cp "$JAR" "$TMP/jar-old"
req POST "$API_URL/api/v1/auth/refresh" -b "$JAR" -c "$JAR"
if [ "$STATUS" = 200 ] && [ -n "$(field access_token)" ] && ! cmp -s "$JAR" "$TMP/jar-old"; then
  pass AUTH-REFRESH "yeni access + yeni refresh cookie"
else fail AUTH-REFRESH "durum=$STATUS"; fi
req POST "$API_URL/api/v1/auth/refresh" -b "$TMP/jar-old"
if [ "$STATUS" = 401 ] && [ "$(field code)" = INVALID_REFRESH_TOKEN ]; then pass AUTH-REFRESH-REUSE "kullanılmış refresh 401"
else fail AUTH-REFRESH-REUSE "durum=$STATUS kod=$(field code)"; fi

# --- F-1 / T-1: teklif, prim motoru, poliçeye çevirme ---
num_field() { sed -n "s/.*\"$1\":\(-\{0,1\}[0-9][0-9]*\).*/\1/p" "$TMP/body" | head -n 1; }
first_num() { grep -o "\"$1\":-\{0,1\}[0-9][0-9]*" "$TMP/body" | head -n 1 | sed 's/.*://'; }
sum_lines() { grep -o '"amount_kurus":-\{0,1\}[0-9][0-9]*' "$TMP/body" | sed 's/.*://' | awk '{s += $1} END {print s + 0}'; }
customer_id() { # maskeli TCKN son 4 hanesine göre müşteri id'si
  grep -o "{\"id\":[0-9]*,\"full_name\":\"[^\"]*\",\"national_id_masked\":\"[*]*$1\"" "$TMP/body" | head -n 1 | sed 's/{"id":\([0-9]*\).*/\1/'
}
START="$(date -v+7d +%F 2>/dev/null || date -d '+7 days' +%F)"
AGENT="${TOKEN_AGENT:-}"
UW="${TOKEN_UNDERWRITER:-}"
req GET "$API_URL/api/v1/customers?limit=100" -H "Authorization: Bearer $AGENT"
AYSE="$(customer_id 0001)"
CAN="$(customer_id 0005)"
quote_body() { # müşteri başlangıç
  printf '{"customer_id":%s,"product":"KASKO","term_start":"%s","ncd_level":2,"vehicle_group":"C","plate":"34ABC123","coverages":["KASKO_ANA","CAM"]}' "$1" "$2"
}
preview() { req POST "$API_URL/api/v1/quotes/preview" -H "Authorization: Bearer $AGENT" -H 'Content-Type: application/json' --data "$(quote_body "$1" "$2")"; }

if [ -z "$AYSE" ] || [ -z "$AGENT" ]; then
  fail "T-1 hazirlik" "demo müşteri/acente bulunamadı"
else
  preview "$AYSE" "$START"; s1="$STATUS"; cp "$TMP/body" "$TMP/p1"
  preview "$AYSE" "$START"; s2="$STATUS"
  if [ "$s1" = 200 ] && [ "$s2" = 200 ] && cmp -s "$TMP/p1" "$TMP/body"; then
    pass "T-1 determinizm" "aynı girdi iki kez → aynı döküm/prim/hash ($(num_field total_premium_kurus) kuruş)"
  else fail "T-1 determinizm" "durum=$s1/$s2 ya da yanıtlar farklı"; fi

  total="$(num_field total_premium_kurus)"
  sum="$(sum_lines)"
  if [ -n "$total" ] && [ "$total" = "$sum" ] && [ "$total" -gt 0 ]; then
    pass "T-1 dokum-toplami" "satır toplamı $sum = prim $total"
  else fail "T-1 dokum-toplami" "satır toplamı=$sum prim=$total"; fi

  preview "$AYSE" 2026-06-30; v_old="$(first_num version_no)"; p_old="$(num_field total_premium_kurus)"
  preview "$AYSE" 2026-07-01; v_new="$(first_num version_no)"; p_new="$(num_field total_premium_kurus)"
  if [ "$v_old" = 1 ] && [ "$v_new" = 2 ] && [ "$p_old" != "$p_new" ]; then
    pass "T-1 tarife-tarihi" "2026-06-30→v1 ($p_old), 2026-07-01→v2 ($p_new)"
  else fail "T-1 tarife-tarihi" "v_old=$v_old v_new=$v_new"; fi

  req POST "$API_URL/api/v1/quotes" -H "Authorization: Bearer $AGENT" -H 'Content-Type: application/json' --data "$(quote_body "$AYSE" "$START")"
  qid="$(first_num id)"; qtotal="$(num_field total_premium_kurus)"; qhash="$(field input_hash)"
  if [ "$STATUS" = 201 ] && [ -n "$qid" ]; then
    req GET "$API_URL/api/v1/quotes/$qid" -H "Authorization: Bearer $AGENT"
    if [ "$STATUS" = 200 ] && [ "$(num_field total_premium_kurus)" = "$qtotal" ] && [ "$(field input_hash)" = "$qhash" ] && [ "$(sum_lines)" = "$qtotal" ]; then
      pass "F-1 teklif-kaydet-oku" "teklif #$qid, $qtotal kuruş, döküm korunuyor"
    else fail "F-1 teklif-kaydet-oku" "okuma durum=$STATUS"; fi
  else fail "F-1 teklif-kaydet-oku" "kayıt durum=$STATUS"; fi

  KEY="verify-$$-$(date +%s)"
  req POST "$API_URL/api/v1/quotes/$qid/convert" -H "Authorization: Bearer $AGENT" -H "Idempotency-Key: $KEY"
  c1="$STATUS"; pno1="$(field policy_no)"
  req POST "$API_URL/api/v1/quotes/$qid/convert" -H "Authorization: Bearer $AGENT" -H "Idempotency-Key: $KEY" -D "$TMP/h2"
  c2="$STATUS"; pno2="$(field policy_no)"
  if [ "$c1" = 201 ] && [ "$c2" = 201 ] && [ -n "$pno1" ] && [ "$pno1" = "$pno2" ] && grep -qi '^idempotent-replayed: true' "$TMP/h2"; then
    pass "F-1 police-cevir" "$pno1; aynı Idempotency-Key tekrarında aynı poliçe"
  else fail "F-1 police-cevir" "durum=$c1/$c2 poliçe=$pno1/$pno2"; fi
  req POST "$API_URL/api/v1/quotes/$qid/convert" -H "Authorization: Bearer $AGENT" -H "Idempotency-Key: $KEY-baska"
  if [ "$STATUS" = 409 ] && [ "$(field code)" = QUOTE_NOT_CONVERTIBLE ]; then
    pass "F-1 cift-cevirme" "farklı anahtarla ikinci çevirme 409"
  else fail "F-1 cift-cevirme" "durum=$STATUS"; fi

  if [ -n "$CAN" ] && [ -n "$UW" ]; then
    req POST "$API_URL/api/v1/quotes" -H "Authorization: Bearer $AGENT" -H 'Content-Type: application/json' --data "$(quote_body "$CAN" "$START")"
    uwq="$(first_num id)"; uwstatus="$(field status)"
    req POST "$API_URL/api/v1/quotes/$uwq/review" -H "Authorization: Bearer $UW" -H 'Content-Type: application/json' \
      --data '{"decision":"APPROVE","note":"verify"}'
    if [ "$uwstatus" = UW_REVIEW ] && [ "$STATUS" = 200 ] && [ "$(field status)" = APPROVED ]; then
      pass "F-1 uw-onay" "genç sürücü → UW_REVIEW → APPROVED"
    else fail "F-1 uw-onay" "ilk durum=$uwstatus onay=$STATUS"; fi
  else fail "F-1 uw-onay" "demo genç sürücü/underwriter yok"; fi
fi

# --- F-2 / T-2: zeyil, tarihte teminat, iptal ---
# Tarih yardımcıları (BSD/macOS ve GNU date)
to_epoch() { date -u -j -f '%Y-%m-%d %H:%M:%S' "$1 00:00:00" +%s 2>/dev/null || date -u -d "$1" +%s; }
add_days() {
  case "$2" in -*) v="$2" ;; *) v="+$2" ;; esac
  date -u -j -v"${v}d" -f '%Y-%m-%d %H:%M:%S' "$1 00:00:00" +%F 2>/dev/null || date -u -d "$1 $2 days" +%F
}
days_between() { echo $(( ($(to_epoch "$2") - $(to_epoch "$1")) / 86400 )); }
# Kuruşa yuvarlama: yarımı yukarı, negatifte sıfırdan uzağa (sunucudaki kuralın bağımsız tekrarı)
round_div() { awk -v n="$1" -v d="$2" 'BEGIN { if (n >= 0) print int((2*n + d) / (2*d)); else print -int((-2*n + d) / (2*d)) }'; }
first_count() { grep -o "$1" "$TMP/body" | wc -l | tr -d ' '; }

if [ -z "$AYSE" ] || [ -z "$AGENT" ]; then
  fail "T-2 hazirlik" "demo müşteri/acente bulunamadı"
else
  req POST "$API_URL/api/v1/quotes" -H "Authorization: Bearer $AGENT" -H 'Content-Type: application/json' --data "$(quote_body "$AYSE" "$START")"
  pq="$(first_num id)"
  req POST "$API_URL/api/v1/quotes/$pq/convert" -H "Authorization: Bearer $AGENT" -H "Idempotency-Key: verify-a3-$$-$pq"
  PID="$(first_num id)"; PS="$(field start_date)"; PE="$(field end_date)"; A1="$(num_field annual_premium_kurus)"
  if [ "$STATUS" != 201 ] || [ -z "$PID" ]; then
    fail "T-2 hazirlik" "poliçe düzenlenemedi durum=$STATUS"
  else
    ZD="$(add_days "$PS" 100)"
    zbody="{\"effective_date\":\"$ZD\",\"vehicle_group\":\"D\",\"plate\":\"06ABC42\",\"base_version_no\":1}"
    req POST "$API_URL/api/v1/policies/$PID/endorsements/preview" -H "Authorization: Bearer $AGENT" -H 'Content-Type: application/json' --data "$zbody"
    old="$(num_field old_annual_premium_kurus)"; new="$(num_field new_annual_premium_kurus)"
    delta="$(num_field premium_delta_kurus)"; rem="$(num_field remaining_days)"; per="$(num_field period_days)"
    want_rem="$(days_between "$ZD" "$PE")"; want_per="$(days_between "$PS" "$PE")"
    want_delta="$(round_div $(( (new - old) * want_rem )) "$want_per")"
    if [ "$STATUS" = 200 ] && [ "$rem" = "$want_rem" ] && [ "$per" = "$want_per" ] && [ "$delta" = "$want_delta" ]; then
      pass "T-2 zeyil-gun-esasli" "($new − $old) × $rem / $per = $delta kuruş (bağımsız hesap: $want_delta)"
    else fail "T-2 zeyil-gun-esasli" "durum=$STATUS kalan=$rem/$want_rem dönem=$per/$want_per delta=$delta/$want_delta"; fi

    ZKEY="verify-z-$$-$PID"
    req POST "$API_URL/api/v1/policies/$PID/endorsements" -H "Authorization: Bearer $AGENT" -H "Idempotency-Key: $ZKEY" -H 'Content-Type: application/json' --data "$zbody"
    # Tekrar yanıtı JSONB'den gelir ve anahtar sırası değişebilir: sıradan bağımsız tekil alanlar karşılaştırılır
    z1="$STATUS"; eid1="$(first_num endorsement_no)"; zdelta="$(num_field premium_delta_kurus)"; zver="$(first_num version_no)"
    req POST "$API_URL/api/v1/policies/$PID/endorsements" -H "Authorization: Bearer $AGENT" -H "Idempotency-Key: $ZKEY" -H 'Content-Type: application/json' --data "$zbody" -D "$TMP/hz"
    z2="$STATUS"; eid2="$(first_num endorsement_no)"
    if [ "$z1" = 201 ] && [ "$z2" = 201 ] && [ "$eid1" = "$eid2" ] && [ "$zdelta" = "$delta" ] && [ "$zver" = 2 ] && grep -qi '^idempotent-replayed: true' "$TMP/hz"; then
      pass "F-2 zeyil-kayit" "zeyil no $eid1 → sürüm 2, delta=$zdelta; aynı anahtar tekrarında aynı zeyil"
    else fail "F-2 zeyil-kayit" "durum=$z1/$z2 zeyil=$eid1/$eid2 delta=$zdelta sürüm=$zver"; fi

    req GET "$API_URL/api/v1/policies/$PID/coverage?date=$(add_days "$ZD" -1)" -H "Authorization: Bearer $AGENT"; c1="$STATUS"; v1="$(first_num version_no)"
    req GET "$API_URL/api/v1/policies/$PID/coverage?date=$ZD" -H "Authorization: Bearer $AGENT"; c2="$STATUS"; v2="$(first_num version_no)"
    if [ "$c1" = 200 ] && [ "$v1" = 1 ] && [ "$c2" = 200 ] && [ "$v2" = 2 ]; then
      pass "T-2 tarihte-teminat" "$(add_days "$ZD" -1) → sürüm 1, $ZD → sürüm 2"
    else fail "T-2 tarihte-teminat" "önceki gün=$c1/v$v1 zeyil günü=$c2/v$v2"; fi

    req GET "$API_URL/api/v1/policies/$PID" -H "Authorization: Bearer $AGENT"
    nv="$(first_count '"effective_from":"')"; ne="$(first_count '"type":"VEHICLE_CHANGE"')"
    if [ "$STATUS" = 200 ] && [ "$nv" = 2 ] && [ "$ne" = 1 ] && grep -q "\"effective_to\":\"$ZD\"" "$TMP/body"; then
      pass "F-2 surum-gecmisi" "2 sürüm (v1 $ZD'de kapandı), 1 zeyil"
    else fail "F-2 surum-gecmisi" "durum=$STATUS sürüm=$nv zeyil=$ne"; fi

    CD="$(add_days "$PS" 200)"
    cbody="{\"effective_date\":\"$CD\",\"reason\":\"CUSTOMER_REQUEST\",\"note\":\"verify\",\"base_version_no\":2}"
    elapsed="$(days_between "$PS" "$CD")"
    # Demo seed kısa dönem tablosu (KURGUSAL): [0,31) %25, [31,91) %40, [91,181) %60, [181,271) %80, [271,367) %100
    if [ "$elapsed" -lt 31 ]; then rate=2500; elif [ "$elapsed" -lt 91 ]; then rate=4000; elif [ "$elapsed" -lt 181 ]; then rate=6000; elif [ "$elapsed" -lt 271 ]; then rate=8000; else rate=10000; fi
    written=$(( A1 + zdelta ))
    earned="$(round_div $(( new * rate )) 10000)"
    want_refund=$(( written - earned )); [ "$want_refund" -lt 0 ] && want_refund=0
    req POST "$API_URL/api/v1/policies/$PID/cancel" -H "Authorization: Bearer $AGENT" -H "Idempotency-Key: verify-c-$$-$PID" -H 'Content-Type: application/json' --data "$cbody"
    refund="$(num_field refund_kurus)"
    if [ "$STATUS" = 201 ] && [ "$refund" = "$want_refund" ] && grep -q '"status":"CANCELLED"' "$TMP/body"; then
      pass "T-2 kisa-donem-iade" "$elapsed gün → %$((rate / 100)); iade = $written − $earned = $refund kuruş"
    else fail "T-2 kisa-donem-iade" "durum=$STATUS iade=$refund beklenen=$want_refund"; fi

    req GET "$API_URL/api/v1/policies/$PID/coverage?date=$(add_days "$CD" -1)" -H "Authorization: Bearer $AGENT"; b1="$STATUS"
    req GET "$API_URL/api/v1/policies/$PID/coverage?date=$CD" -H "Authorization: Bearer $AGENT"; b2="$STATUS"; b2code="$(field code)"
    req POST "$API_URL/api/v1/policies/$PID/endorsements" -H "Authorization: Bearer $AGENT" -H "Idempotency-Key: verify-z2-$$-$PID" -H 'Content-Type: application/json' \
      --data "{\"effective_date\":\"$(add_days "$CD" 5)\",\"vehicle_group\":\"A\"}"
    b3="$STATUS"; b3code="$(field code)"
    if [ "$b1" = 200 ] && [ "$b2" = 404 ] && [ "$b2code" = NO_COVERAGE_ON_DATE ] && [ "$b3" = 422 ] && [ "$b3code" = POLICY_CANCELLED ]; then
      pass "F-2 iptal-sonrasi" "iptalden bir gün önce teminat var; $CD'de 404, zeyil 422"
    else fail "F-2 iptal-sonrasi" "önce=$b1 iptal günü=$b2/$b2code zeyil=$b3/$b3code"; fi
  fi
fi

# --- F-4 / F-5 / F-6 / T-3: hasar, ekspertiz, tazminat ve gerçek paralel onay ---
TODAY="$(TZ=Europe/Istanbul date +%F)"
ADJ="${TOKEN_CLAIMS_ADJUSTER:-}"
APPRAISER="${TOKEN_APPRAISER:-}"
ACCOUNTING="${TOKEN_ACCOUNTING:-}"
A4KEY="verify-a4-$$-$(date +%s)"

claim_policy() { # ürün; POLICY değişkenini doldurur
  local body
  if [ "$1" = KASKO_RACE ]; then
    body="{\"customer_id\":$AYSE,\"product\":\"KASKO\",\"term_start\":\"$TODAY\",\"ncd_level\":2,\"vehicle_group\":\"C\",\"plate\":\"34ABC123\",\"coverages\":[\"KASKO_ANA\",\"IMM\"]}"
  else body="$(quote_body "$AYSE" "$TODAY")"; fi
  req POST "$API_URL/api/v1/quotes" -H "Authorization: Bearer $AGENT" -H 'Content-Type: application/json' --data "$body"
  local q="$(first_num id)"
  if [ "$(field status)" = UW_REVIEW ]; then
    req POST "$API_URL/api/v1/quotes/$q/review" -H "Authorization: Bearer $UW" -H 'Content-Type: application/json' --data '{"decision":"APPROVE","note":"verify A4"}'
  fi
  req POST "$API_URL/api/v1/quotes/$q/convert" -H "Authorization: Bearer $AGENT" -H "Idempotency-Key: $A4KEY-policy-$q"
  POLICY="$(first_num id)"
  [ "$STATUS" = 201 ] && [ -n "$POLICY" ]
}
ready_claim() { # poliçe, teminat, brüt; READY_CLAIM / READY_OK
  READY_OK=yes
  READY_SEQ=$(( ${READY_SEQ:-0} + 1 ))
  req POST "$API_URL/api/v1/claims" -H "Authorization: Bearer $AGENT" -H "Idempotency-Key: $A4KEY-report-$1-$2-$READY_SEQ" -H 'Content-Type: application/json' \
    --data "{\"policy_id\":$1,\"coverage_code\":\"$2\",\"loss_date\":\"$TODAY\",\"description\":\"verify A4\",\"claimed_amount_kurus\":$3}"
  READY_CLAIM="$(first_num id)"
  [ "$STATUS" = 201 ] && body_has '"row_version":1' || READY_OK=no
  req POST "$API_URL/api/v1/claims/$READY_CLAIM/transition" -H "Authorization: Bearer $ADJ" -H 'Content-Type: application/json' --data '{"to_status":"TRIAGED","expected_row_version":1}'
  [ "$STATUS" = 200 ] || READY_OK=no
  req POST "$API_URL/api/v1/claims/$READY_CLAIM/transition" -H "Authorization: Bearer $ADJ" -H 'Content-Type: application/json' --data '{"to_status":"APPRAISAL","expected_row_version":2}'
  [ "$STATUS" = 200 ] || READY_OK=no
  req POST "$API_URL/api/v1/claims/$READY_CLAIM/appraisals" -H "Authorization: Bearer $APPRAISER" -H 'Content-Type: application/json' \
    --data "{\"damage_amount_kurus\":$3,\"is_total_loss\":false,\"notes\":\"verify\",\"inspected_at\":\"${TODAY}T10:00:00+03:00\"}"
  [ "$STATUS" = 201 ] || READY_OK=no
  req POST "$API_URL/api/v1/claims/$READY_CLAIM/transition" -H "Authorization: Bearer $ADJ" -H 'Content-Type: application/json' --data '{"to_status":"ADJUSTMENT","expected_row_version":3}'
  [ "$STATUS" = 200 ] || READY_OK=no
}
if claim_policy KASKO; then
  ready_claim "$POLICY" CAM 2000000
  CID="$READY_CLAIM"
  req GET "$API_URL/api/v1/claims/$CID" -H "Authorization: Bearer $AGENT"
  if [ "$READY_OK" = yes ] && [ "$STATUS" = 200 ] && body_has '"to_status":"REPORTED"' && body_has '"to_status":"ADJUSTMENT"'; then
    pass "F-4 hasar-dosya" "hasar #$CID; bildirim → triage → ekspertiz → değerlendirme, geçmiş kayıtlı"
  else fail "F-4 hasar-dosya" "durum=$STATUS hazırlık=$READY_OK"; fi
  if body_has '"damage_amount_kurus":2000000' && body_has '"notes":"verify"'; then
    pass "F-5 ekspertiz" "gerçek ekspertiz kaydı: 2000000 kuruş"
  else fail "F-5 ekspertiz" "ekspertiz detayda bulunamadı"; fi
  req POST "$API_URL/api/v1/claims/$CID/settlement/preview" -H "Authorization: Bearer $ADJ"
  if [ "$STATUS" = 200 ] && [ "$(num_field payable_kurus)" = 1950000 ] && body_has '"code":"DEDUCTIBLE"' && body_has '"code":"ANNUAL_LIMIT"'; then
    pass "T-3 hesap-adimlari" "2000000 − 50000 sabit muafiyet = 1950000 kuruş, altı hesap adımı"
  else fail "T-3 hesap-adimlari" "durum=$STATUS ödeme=$(num_field payable_kurus)"; fi
  req POST "$API_URL/api/v1/claims/$CID/settlement/approve" -H "Authorization: Bearer $ADJ" -H "Idempotency-Key: $A4KEY-approve-$CID"
  SID="$(first_num id)"; approval_status="$STATUS"
  req POST "$API_URL/api/v1/claims/$CID/settlement/approve" -H "Authorization: Bearer $ADJ" -H "Idempotency-Key: $A4KEY-approve-$CID" -D "$TMP/a4-idem"
  if [ "$approval_status" = 201 ] && [ "$STATUS" = 201 ] && [ "$SID" = "$(first_num id)" ] && grep -qi '^idempotent-replayed: true' "$TMP/a4-idem"; then
    pass "F-6 idempotent-onay" "tek tazminat #$SID; aynı anahtarda aynı kayıt"
  else fail "F-6 idempotent-onay" "durum=$approval_status/$STATUS"; fi
  req POST "$API_URL/api/v1/claims/$CID/pay" -H "Authorization: Bearer $ACCOUNTING" -H "Idempotency-Key: $A4KEY-pay-$CID"
  if [ "$STATUS" = 200 ] && [ "$(field status)" = PAID ] && [ "$(num_field payable_kurus)" = 1950000 ] && [ -n "$(num_field payment_journal_entry_id)" ]; then
    pass "F-6 odeme" "1950000 kuruş ödendi; yevmiye #$(num_field payment_journal_entry_id)"
  else fail "F-6 odeme" "durum=$STATUS"; fi
else fail "F-4 hazirlik" "bugün başlayan poliçe oluşturulamadı"; fi

if claim_policy KASKO_RACE; then
  RACE_POLICY="$POLICY"
  ready_claim "$RACE_POLICY" KASKO_ANA 150000000; C1="$READY_CLAIM"; R1="$READY_OK"
  ready_claim "$RACE_POLICY" IMM 50000000; C2="$READY_CLAIM"; R2="$READY_OK"
  curl -sS --max-time 15 -o "$TMP/race1" -w '%{http_code}' -X POST -H "Authorization: Bearer $ADJ" -H "Idempotency-Key: $A4KEY-race-$C1" "$API_URL/api/v1/claims/$C1/settlement/approve" >"$TMP/race1-status" &
  p1=$!
  curl -sS --max-time 15 -o "$TMP/race2" -w '%{http_code}' -X POST -H "Authorization: Bearer $ADJ" -H "Idempotency-Key: $A4KEY-race-$C2" "$API_URL/api/v1/claims/$C2/settlement/approve" >"$TMP/race2-status" &
  p2=$!
  wait "$p1"; wait "$p2"
  paid1="$(sed -n 's/.*"payable_kurus":\([0-9]*\).*/\1/p' "$TMP/race1")"
  paid2="$(sed -n 's/.*"payable_kurus":\([0-9]*\).*/\1/p' "$TMP/race2")"
  req GET "$API_URL/api/v1/policies/$RACE_POLICY" -H "Authorization: Bearer $ADJ"
  annual="$(grep -o '"coverage_code":null[^}]*}' "$TMP/body")"
  used="$(printf '%s' "$annual" | sed -n 's/.*"used_kurus":\([0-9]*\).*/\1/p')"
  if [ "$R1" = yes ] && [ "$R2" = yes ] && [ "$C1" != "$C2" ] && [ "$(cat "$TMP/race1-status")" = 201 ] && [ "$(cat "$TMP/race2-status")" = 201 ] && [ "$used" = 180000000 ] && [ "$(( ${paid1:-0} + ${paid2:-0} ))" = 180000000 ]; then
    pass "T-3 paralel-limit" "iki paralel onay: $paid1 + $paid2 = $used ≤ 180000000 yıllık limit"
  else fail "T-3 paralel-limit" "durum=$(cat "$TMP/race1-status")/$(cat "$TMP/race2-status") tutar=$paid1/$paid2 kullanım=$used"; fi
else fail "T-3 hazirlik" "yarış poliçesi oluşturulamadı"; fi

# --- nginx üzerinden aynı origin akışı ---
req POST "$WEB_URL/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data "{\"email\":\"acente@guvenpolice.test\",\"password\":\"$DEMO_PASSWORD\"}" -D "$TMP/headers"
if [ "$STATUS" = 200 ] && grep -qi '^set-cookie: gp_refresh=.*HttpOnly' "$TMP/headers"; then
  pass WEB-PROXY-LOGIN ":3000/api üzerinden giriş + httpOnly cookie"
else fail WEB-PROXY-LOGIN "durum=$STATUS"; fi

echo
echo "Henüz uygulanmamış şartname kimlikleri (sonraki aşamalar):"
for id in F-3 F-7 T-4 T-5; do
  printf '  YAPILMADI  %s\n' "$id"
done

echo
echo "Sonuç: $PASSED PASS, $FAILED FAIL"
[ "$FAILED" -eq 0 ]
