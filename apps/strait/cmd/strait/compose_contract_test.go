package main

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestComposeRuntimeContractIncludesRedisAndSequin(t *testing.T) {
	t.Parallel()

	base := readRepoFile(t, "../../../../docker-compose.base.yml")
	for _, want := range []string{
		"REDIS_URL: redis://redis:6379",
		"SEQUIN_BASE_URL: http://sequin:7376",
		"SEQUIN_CONSUMER_NAME: strait-cdc",
		"SEQUIN_API_TOKEN: ${SEQUIN_API_TOKEN:-local-dev-sequin-api-token-change-me}",
		"redis:",
		"image: redis:8-alpine",
		"test: [\"CMD\", \"redis-cli\", \"ping\"]",
		"postgres-cdc-init:",
		"psql \"$$DATABASE_URL\" -v ON_ERROR_STOP=1 -f /config/postgres-init.sql",
		"sequin:",
		"image: sequin/sequin:v0.14.6",
		"CONFIG_FILE_PATH: /etc/sequin/config.yml",
		"./packages/configs/sequin.yml:/etc/sequin/config.yml:ro",
		"test: [\"CMD-SHELL\", \"curl -sf http://localhost:7376/health || exit 1\"]",
		"condition: service_completed_successfully",
	} {
		requireContains(t, base, want)
	}

	requireServiceDependency(t, base, "strait-migrate:", "redis:", "condition: service_healthy")
	requireServiceDependency(t, base, "postgres-cdc-init:", "strait-migrate:", "condition: service_completed_successfully")
	requireServiceDependency(t, base, "sequin:", "postgres:", "condition: service_healthy")
	requireServiceDependency(t, base, "sequin:", "redis:", "condition: service_healthy")
	requireServiceDependency(t, base, "sequin:", "postgres-cdc-init:", "condition: service_completed_successfully")
	requireServiceDependency(t, base, "strait:", "redis:", "condition: service_healthy")
	requireServiceDependency(t, base, "strait:", "sequin:", "condition: service_healthy")

	selfhost := readRepoFile(t, "../../../../docker-compose.selfhost.yml")
	for _, want := range []string{
		"SEQUIN_API_TOKEN: ${SEQUIN_API_TOKEN:?run ./packages/scripts/selfhost-init.sh}",
		"SECRET_KEY_BASE: ${SEQUIN_SECRET_KEY_BASE:?run ./packages/scripts/selfhost-init.sh}",
		"VAULT_KEY: ${SEQUIN_VAULT_KEY:?run ./packages/scripts/selfhost-init.sh}",
		"STRAIT_EDITION: community",
	} {
		requireContains(t, selfhost, want)
	}

	dev := readRepoFile(t, "../../docker-compose.yml")
	for _, want := range []string{
		"redis:",
		"- \"16379:6379\"",
		"sequin:",
		"- \"7376:7376\"",
		"STRAIT_ENV: development",
	} {
		requireContains(t, dev, want)
	}
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()

	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(raw)
}

func requireContains(t *testing.T, source, want string) {
	t.Helper()
	require.Contains(t, source, want)
}

func requireServiceDependency(t *testing.T, source, service, dependency, condition string) {
	t.Helper()

	serviceBlock := indentedBlock(source, "  "+service, "  ")
	require.NotEmpty(t,
		serviceBlock,
	)

	dependencyBlock := indentedBlock(serviceBlock, "      "+dependency, "      ")
	require.NotEmpty(t,
		dependencyBlock,
	)
	require.Contains(t, dependencyBlock, condition)
}

func indentedBlock(source, header, siblingIndent string) string {
	lines := strings.Split(source, "\n")
	start := -1
	for i, line := range lines {
		if line == header {
			start = i
			break
		}
	}
	if start == -1 {
		return ""
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, siblingIndent) && !strings.HasPrefix(line, siblingIndent+" ") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}
