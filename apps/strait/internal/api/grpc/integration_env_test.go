//go:build integration

package grpc

import (
	"context"
	"log"
	"os"
	"testing"

	"strait/internal/testutil"
)

var grpcTestEnv *testutil.TestEnv

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	grpcTestEnv, err = testutil.SetupSharedTestEnv(ctx, "../../../migrations", "api-grpc")
	if err != nil {
		log.Fatalf("setup grpc test env: %v", err)
	}

	code := m.Run()
	grpcTestEnv.Cleanup(ctx)
	os.Exit(code)
}

func cleanIntegrationEnv(t *testing.T, ctx context.Context) *testutil.TestEnv {
	t.Helper()
	if grpcTestEnv == nil {
		t.Fatal("grpc test env is not initialized")
	}
	if err := grpcTestEnv.Clean(ctx); err != nil {
		t.Fatalf("clean test env: %v", err)
	}
	return grpcTestEnv
}
