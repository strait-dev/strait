#!/usr/bin/env bash
set -euo pipefail

# Verify the latest etcd backup snapshot is valid.
# Downloads from S3 and checks integrity with etcdutl.
#
# Usage: ./verify-backup.sh
#
# Required environment variables (from Doppler or /etc/default/etcd-backup):
#   BACKUP_S3_BUCKET    — S3 bucket name
#   BACKUP_S3_ENDPOINT  — S3 endpoint URL
#   AWS_ACCESS_KEY_ID   — S3 access key
#   AWS_SECRET_ACCESS_KEY — S3 secret key
#
# Exit codes:
#   0 = backup is valid
#   1 = no backup found or verification failed

BUCKET="${BACKUP_S3_BUCKET:?BACKUP_S3_BUCKET is required}"
ENDPOINT="${BACKUP_S3_ENDPOINT:?BACKUP_S3_ENDPOINT is required}"
TMP_DIR=$(mktemp -d)
trap "rm -rf $TMP_DIR" EXIT

echo "Listing snapshots in s3://${BUCKET}/etcd-snapshots/..."
LATEST=$(aws s3 ls "s3://${BUCKET}/etcd-snapshots/" \
  --endpoint-url "${ENDPOINT}" \
  | sort -k1,2 \
  | tail -1 \
  | awk '{print $4}')

if [ -z "$LATEST" ]; then
  echo "ERROR: No snapshots found in s3://${BUCKET}/etcd-snapshots/"
  exit 1
fi

echo "Downloading latest snapshot: ${LATEST}"
aws s3 cp "s3://${BUCKET}/etcd-snapshots/${LATEST}" "${TMP_DIR}/${LATEST}" \
  --endpoint-url "${ENDPOINT}" \
  --quiet

echo "Verifying snapshot integrity..."
if command -v etcdutl &>/dev/null; then
  etcdutl snapshot status "${TMP_DIR}/${LATEST}" --write-out=table
elif command -v etcdctl &>/dev/null; then
  ETCDCTL_API=3 etcdctl snapshot status "${TMP_DIR}/${LATEST}" --write-out=table
else
  echo "WARNING: etcdutl/etcdctl not found, checking file size only"
  SIZE=$(stat -f%z "${TMP_DIR}/${LATEST}" 2>/dev/null || stat -c%s "${TMP_DIR}/${LATEST}")
  if [ "$SIZE" -lt 1000 ]; then
    echo "ERROR: Snapshot too small (${SIZE} bytes), likely corrupt"
    exit 1
  fi
  echo "Snapshot size: ${SIZE} bytes (looks reasonable)"
fi

echo ""
echo "Backup verification passed: ${LATEST}"
