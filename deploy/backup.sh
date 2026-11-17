#!/bin/sh
# Daily backup of the database and uploaded files. Run from cron on the server:
#   15 3 * * * /opt/healthcare/deploy/backup.sh >> /var/log/healthcare-backup.log 2>&1
#
# Keeps 14 days locally. Copy BACKUP_DIR to another machine or cloud storage as
# well: a backup on the same disk does not survive losing the server.
set -eu

cd "$(dirname "$0")"
BACKUP_DIR="${BACKUP_DIR:-/var/backups/healthcare}"
STAMP="$(date +%Y%m%d-%H%M%S)"
COMPOSE="docker compose -f docker-compose.prod.yml"

mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"

# Consistent database dump in PostgreSQL's compressed custom format.
$COMPOSE exec -T db pg_dump -U healthcare -Fc healthcare > "$BACKUP_DIR/db-$STAMP.dump"

# Uploaded reports, prescriptions and photos (the API image has no shell, so a
# small helper container reads the volume).
docker run --rm -v healthcare_uploads:/data:ro -v "$BACKUP_DIR:/backup" alpine:3.20 \
  tar -czf "/backup/uploads-$STAMP.tar.gz" -C /data .

find "$BACKUP_DIR" -type f -mtime +14 -delete
echo "$(date -Is) backup ok: $STAMP"

# Restore (on a fresh stack):
#   docker compose -f docker-compose.prod.yml exec -T db pg_restore -U healthcare -d healthcare --clean < db-STAMP.dump
#   docker run --rm -v healthcare_uploads:/data -v "$BACKUP_DIR:/backup" alpine:3.20 tar -xzf /backup/uploads-STAMP.tar.gz -C /data
