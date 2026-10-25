#!/usr/bin/env bash
# Geçici bir PostgreSQL 16 konteyneri açıp integration build tag'li Go testlerini çalıştırır.
# DATABASE_URL verilmişse konteyner açmaz, onu kullanır.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NAME="guvenpolice-it-$$"

if [ -z "${DATABASE_URL:-}" ]; then
  docker run -d --rm --name "$NAME" -e POSTGRES_PASSWORD=it -e POSTGRES_USER=it -e POSTGRES_DB=it \
    -p 127.0.0.1::5432 postgres:16-alpine >/dev/null
  trap 'docker rm -f "$NAME" >/dev/null 2>&1 || true' EXIT
  PORT="$(docker port "$NAME" 5432/tcp | head -1 | sed 's/.*://')"
  for _ in $(seq 1 60); do
    if docker exec "$NAME" pg_isready -U it -d it >/dev/null 2>&1; then break; fi
    sleep 1
  done
  # pg_isready ilk açılıştaki geçici sunucuda da hazır diyebilir; gerçek bağlantıyı bekle
  for _ in $(seq 1 30); do
    if docker exec "$NAME" psql -U it -d it -c 'SELECT 1' >/dev/null 2>&1; then break; fi
    sleep 1
  done
  export DATABASE_URL="postgres://it:it@127.0.0.1:${PORT}/it?sslmode=disable"
fi

cd "$ROOT/backend"
go test -tags integration -count=1 "$@" ./...
