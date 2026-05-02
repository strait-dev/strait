#!/bin/sh
set -e

BACKUP_DIR="${BACKUP_DIR:-./backups}"
CONTAINER="${CONTAINER:-strait-postgres}"

mkdir -p "$BACKUP_DIR"

TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="$BACKUP_DIR/strait_$TIMESTAMP.sql.gz"

echo "Backing up Strait database..."
docker exec "$CONTAINER" pg_dump -U strait strait | gzip > "$BACKUP_FILE"

SIZE=$(du -h "$BACKUP_FILE" | cut -f1)
echo "Backup saved to $BACKUP_FILE ($SIZE)"

# Clean up backups older than 30 days.
find "$BACKUP_DIR" -name "strait_*.sql.gz" -mtime +30 -delete 2>/dev/null || true
