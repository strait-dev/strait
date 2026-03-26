#!/bin/sh
set -e

ENV_FILE=".env.selfhost"

# --reset: wipe everything and start fresh.
if [ "$1" = "--reset" ]; then
    echo "Resetting self-hosted deployment..."
    docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml down -v 2>/dev/null || true
    rm -f "$ENV_FILE"
    echo "Reset complete. Run this script again to generate new secrets."
    exit 0
fi

# Check prerequisites.
if ! docker info >/dev/null 2>&1; then
    echo "Error: Docker is not running. Start Docker and try again."
    exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
    echo "Error: Docker Compose v2 is required but not found."
    echo "Install it: https://docs.docker.com/compose/install/"
    exit 1
fi

# Skip if secrets already exist.
if [ -f "$ENV_FILE" ]; then
    echo "Secrets already exist at $ENV_FILE (use --reset to regenerate)."
    echo ""
    echo "Start Strait:"
    echo "  docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml up -d"
    echo ""
    echo "  Dashboard: http://localhost:3000"
    echo "  API:       http://localhost:8080/health"
    echo "  API docs:  http://localhost:8080/reference"
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
echo "Next steps:"
echo "  docker compose --env-file .env.selfhost -f docker-compose.selfhost.yml up -d"
echo ""
echo "  Dashboard: http://localhost:3000"
echo "  API:       http://localhost:8080/health"
echo "  API docs:  http://localhost:8080/reference"
