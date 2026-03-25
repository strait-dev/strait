#!/bin/sh
set -e

ENV_FILE=".env.selfhost"

if [ -f "$ENV_FILE" ]; then
    echo "Secrets file already exists at $ENV_FILE, skipping generation."
    echo ""
    echo "Your INTERNAL_SECRET (use this for API calls):"
    grep INTERNAL_SECRET "$ENV_FILE" | cut -d= -f2
    exit 0
fi

echo "Generating secrets for Strait self-hosted deployment..."

gen_hex() { head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n'; }
gen_base64() { head -c 32 /dev/urandom | base64 | tr -d '\n'; }

INTERNAL_SECRET=$(gen_hex)
JWT_SIGNING_KEY=$(gen_hex)
ENCRYPTION_KEY=$(gen_base64)
SECRET_ENCRYPTION_KEY=$(gen_base64)
BETTER_AUTH_SECRET=$(gen_hex)
SEQUIN_SECRET_KEY_BASE=$(gen_hex)$(gen_hex)
SEQUIN_VAULT_KEY=$(gen_base64)

cat > "$ENV_FILE" <<EOF
# Auto-generated secrets for Strait self-hosted deployment.
# Do NOT commit this file to version control.

# Strait API secrets
INTERNAL_SECRET=${INTERNAL_SECRET}
JWT_SIGNING_KEY=${JWT_SIGNING_KEY}
ENCRYPTION_KEY=${ENCRYPTION_KEY}
SECRET_ENCRYPTION_KEY=${SECRET_ENCRYPTION_KEY}

# Dashboard (Better Auth) secrets
BETTER_AUTH_SECRET=${BETTER_AUTH_SECRET}

# Sequin secrets
SEQUIN_SECRET_KEY_BASE=${SEQUIN_SECRET_KEY_BASE}
SEQUIN_VAULT_KEY=${SEQUIN_VAULT_KEY}
EOF

echo "Secrets written to $ENV_FILE"
echo ""
echo "Your INTERNAL_SECRET (use this for API calls):"
echo "  $INTERNAL_SECRET"
echo ""
echo "Next steps:"
echo "  docker compose -f docker-compose.selfhost.yml up -d"
echo "  API:       http://localhost:8080/health"
echo "  Dashboard: http://localhost:3000"
