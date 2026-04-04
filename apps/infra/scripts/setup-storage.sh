#!/usr/bin/env bash
set -euo pipefail

# Create S3 buckets on Hetzner Object Storage for Strait infrastructure.
#
# Usage: ./setup-storage.sh
#
# Required environment variables:
#   BACKUP_S3_ENDPOINT      — Hetzner S3 endpoint (e.g., https://fsn1.your-objectstorage.com)
#   AWS_ACCESS_KEY_ID       — Hetzner Object Storage access key
#   AWS_SECRET_ACCESS_KEY   — Hetzner Object Storage secret key
#
# Buckets created:
#   strait-backups    — etcd snapshot backups (synced every 6h from master)
#   strait-tf-state   — Terraform remote state

ENDPOINT="${BACKUP_S3_ENDPOINT:?BACKUP_S3_ENDPOINT is required}"

create_bucket() {
  local bucket="$1"
  echo "Creating bucket: $bucket..."
  if aws s3api head-bucket --bucket "$bucket" --endpoint-url "$ENDPOINT" 2>/dev/null; then
    echo "  Already exists"
  else
    aws s3api create-bucket --bucket "$bucket" --endpoint-url "$ENDPOINT"
    echo "  Created"
  fi

  # Enable versioning for state protection.
  aws s3api put-bucket-versioning \
    --bucket "$bucket" \
    --versioning-configuration Status=Enabled \
    --endpoint-url "$ENDPOINT" 2>/dev/null || echo "  Versioning not supported (Hetzner limitation)"
}

create_bucket "strait-backups"
create_bucket "strait-tf-state"

echo ""
echo "All buckets ready at $ENDPOINT"
echo ""
echo "Next steps:"
echo "  1. Add these to terraform.tfvars or Doppler:"
echo "     backup_s3_endpoint = \"$ENDPOINT\""
echo "     backup_s3_bucket   = \"strait-backups\""
echo "  2. Configure Terraform backend in backend.hcl:"
echo "     bucket   = \"strait-tf-state\""
echo "     endpoint = \"$ENDPOINT\""
