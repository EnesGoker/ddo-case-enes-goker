#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [ -z "${DEMO_PASSWORD:-}" ]; then
  DEMO_PASSWORD="$(sed -n 's/^DEMO_PASSWORD=//p' .env | tail -1 | sed 's/^"\(.*\)"$/\1/')"
fi
export DEMO_PASSWORD
mkdir -p docs/load
# Resmî Grafana imajı; host kurulumu gerekmez. Sadece bu proje ağı kullanılır.
# Sonuçlar kimlik bilgisi/yanıt gövdesi içermez; çalışma doğrudan yereldir.
docker run --rm --network guvenpolice_default --user "$(id -u):$(id -g)" \
  -e DEMO_PASSWORD -e API_URL="${LOAD_API_URL:-http://api:8080}" \
  -e K6_NO_USAGE_REPORT=true -e K6_NO_COLOR=true \
  -v "$PWD/scripts/k6:/scripts:ro" -v "$PWD/docs/load:/results" \
  grafana/k6:2.2.0@sha256:9bd01d6941fca969cb61bb57d2da5ee9b385fe2aa8881df3798c196564d6ace6 run /scripts/load.js
