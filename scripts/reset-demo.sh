#!/usr/bin/env bash
# Demo veritabanını seed durumuna döndürür: iş verisi tablolarını boşaltır (şema korunur),
# ardından API'yi yeniden başlatıp açılış seed'inin tekrar çalışmasını sağlar.
# Migrasyonun kendisinin doldurduğu başvuru tabloları (accounts, claim_transitions) korunur;
# yeni bir migrasyon başvuru verisi eklerse KEEP listesine eklenmelidir.
set -euo pipefail
cd "$(dirname "$0")/.."

# Canlı/batch simülatör veri temizliği sırasında yeniden kayıt eklememeli.
if [ "$(docker inspect -f '{{.State.Running}}' guvenpolice-simulator 2>/dev/null || true)" = true ]; then
  docker stop -t 15 guvenpolice-simulator >/dev/null
fi

KEEP="'goose_db_version','accounts','claim_transitions'"

docker compose exec -T db sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<SQL
DO \$\$
DECLARE t text;
BEGIN
  SELECT string_agg(format('%I.%I', schemaname, tablename), ', ') INTO t
    FROM pg_tables WHERE schemaname = 'public' AND tablename NOT IN ($KEEP);
  EXECUTE 'TRUNCATE ' || t || ' RESTART IDENTITY CASCADE';
END
\$\$;
SQL

docker compose restart api
docker compose up -d --wait api
echo "Demo verisi sıfırlandı ve yeniden yüklendi."
