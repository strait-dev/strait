#!/usr/bin/env bash
set -euo pipefail

# Test etcd backup restore by spinning up a temporary k3s server.
# This is the gold standard for backup verification — not just checking
# file integrity, but actually restoring data and validating it exists.
#
# Usage: ./test-restore.sh
#
# Required:
#   - Docker installed and running
#   - S3 credentials (BACKUP_S3_BUCKET, BACKUP_S3_ENDPOINT, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
#
# Exit codes:
#   0 = restore successful, data validated
#   1 = restore failed or data missing

BUCKET="${BACKUP_S3_BUCKET:?BACKUP_S3_BUCKET is required}"
ENDPOINT="${BACKUP_S3_ENDPOINT:?BACKUP_S3_ENDPOINT is required}"
CONTAINER_NAME="strait-restore-test-$$"
TMP_DIR=$(mktemp -d)
trap "docker rm -f $CONTAINER_NAME 2>/dev/null; rm -rf $TMP_DIR" EXIT

echo "=== Strait Backup Restore Test ==="
echo ""

# 1. Download latest snapshot.
echo "Step 1: Downloading latest snapshot..."
LATEST=$(aws s3 ls "s3://${BUCKET}/etcd-snapshots/" \
  --endpoint-url "${ENDPOINT}" \
  | sort -k1,2 | tail -1 | awk '{print $4}')

if [ -z "$LATEST" ]; then
  echo "FAIL: No snapshots found in s3://${BUCKET}/etcd-snapshots/"
  exit 1
fi

aws s3 cp "s3://${BUCKET}/etcd-snapshots/${LATEST}" "${TMP_DIR}/${LATEST}" \
  --endpoint-url "${ENDPOINT}" --quiet
echo "  Downloaded: ${LATEST} ($(stat -f%z "${TMP_DIR}/${LATEST}" 2>/dev/null || stat -c%s "${TMP_DIR}/${LATEST}") bytes)"

# 2. Verify snapshot integrity.
echo "Step 2: Verifying snapshot integrity..."
if command -v etcdutl &>/dev/null; then
  etcdutl snapshot status "${TMP_DIR}/${LATEST}" --write-out=table
elif command -v etcdctl &>/dev/null; then
  ETCDCTL_API=3 etcdctl snapshot status "${TMP_DIR}/${LATEST}" --write-out=table
else
  echo "  WARN: etcdutl/etcdctl not found, skipping integrity check"
fi

# 3. Start temporary k3s server with restored snapshot.
echo "Step 3: Starting temporary k3s with restored snapshot..."
docker run -d --name "$CONTAINER_NAME" \
  --privileged \
  -v "${TMP_DIR}/${LATEST}:/snapshot.db:ro" \
  -p 6444:6443 \
  rancher/k3s:latest \
  server --cluster-init \
  --cluster-reset \
  --cluster-reset-restore-path=/snapshot.db \
  --disable=traefik \
  --disable=servicelb

# Wait for API server to be ready.
echo "  Waiting for API server (max 120s)..."
for i in $(seq 1 24); do
  if docker exec "$CONTAINER_NAME" kubectl get nodes 2>/dev/null; then
    break
  fi
  if [ "$i" -eq 24 ]; then
    echo "FAIL: k3s API server did not start within 120s"
    docker logs "$CONTAINER_NAME" | tail -20
    exit 1
  fi
  sleep 5
done

# 4. Validate restored data.
echo ""
echo "Step 4: Validating restored data..."
PASSED=true

# Check that the strait namespace exists.
if docker exec "$CONTAINER_NAME" kubectl get namespace strait 2>/dev/null; then
  echo "  [PASS] Namespace 'strait' exists"
else
  echo "  [FAIL] Namespace 'strait' not found"
  PASSED=false
fi

# Check that the strait deployment exists.
if docker exec "$CONTAINER_NAME" kubectl get deployment strait -n strait 2>/dev/null; then
  echo "  [PASS] Deployment 'strait' exists in namespace 'strait'"
else
  echo "  [FAIL] Deployment 'strait' not found in namespace 'strait'"
  PASSED=false
fi

# Check that network policies exist.
NP_COUNT=$(docker exec "$CONTAINER_NAME" kubectl get networkpolicy -n strait --no-headers 2>/dev/null | wc -l)
if [ "$NP_COUNT" -gt 0 ]; then
  echo "  [PASS] $NP_COUNT NetworkPolicies found in namespace 'strait'"
else
  echo "  [FAIL] No NetworkPolicies in namespace 'strait'"
  PASSED=false
fi

# 5. Report results.
echo ""
if [ "$PASSED" = true ]; then
  echo "=== RESTORE TEST PASSED ==="
  echo "Snapshot: ${LATEST}"
  echo "All critical resources validated successfully."
else
  echo "=== RESTORE TEST FAILED ==="
  echo "Some resources were missing after restore. Check snapshot age and completeness."
  exit 1
fi
