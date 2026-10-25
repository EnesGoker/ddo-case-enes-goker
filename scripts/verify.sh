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

pass() { PASSED=$((PASSED + 1)); printf '  PASS  %-26s %s\n' "$1" "$2"; }
fail() { FAILED=$((FAILED + 1)); printf '  FAIL  %-26s %s\n' "$1" "$2"; }

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

# --- nginx üzerinden aynı origin akışı ---
req POST "$WEB_URL/api/v1/auth/login" -H 'Content-Type: application/json' \
  --data "{\"email\":\"acente@guvenpolice.test\",\"password\":\"$DEMO_PASSWORD\"}" -D "$TMP/headers"
if [ "$STATUS" = 200 ] && grep -qi '^set-cookie: gp_refresh=.*HttpOnly' "$TMP/headers"; then
  pass WEB-PROXY-LOGIN ":3000/api üzerinden giriş + httpOnly cookie"
else fail WEB-PROXY-LOGIN "durum=$STATUS"; fi

echo
echo "Henüz uygulanmamış şartname kimlikleri (sonraki aşamalar):"
for id in F-1 F-2 F-3 F-4 F-5 F-6 F-7 T-1 T-2 T-3 T-4 T-5; do
  printf '  YAPILMADI  %s\n' "$id"
done

echo
echo "Sonuç: $PASSED PASS, $FAILED FAIL"
[ "$FAILED" -eq 0 ]
