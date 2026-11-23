//go:build e2e

package integration

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Stone-afk/sol-scanner-service/internal/pkg/postgre"
	"gorm.io/gorm"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func migrationDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(repositoryRoot(t), "scripts", "postgre")
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("SOL_SCANNER_E2E_DSN")
	if dsn == "" {
		t.Fatal("SOL_SCANNER_E2E_DSN is required")
	}
	db, err := postgre.Open(dsn)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		if err := postgre.Close(db); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	if err := postgre.Ping(context.Background(), db); err != nil {
		t.Fatalf("ping test database: %v", err)
	}
	return db
}

func kafkaBrokers(t *testing.T) string {
	t.Helper()
	brokers := os.Getenv("SOL_SCANNER_E2E_KAFKA")
	if brokers == "" {
		t.Fatal("SOL_SCANNER_E2E_KAFKA is required")
	}
	return brokers
}

func resetSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	runner, err := postgre.NewMigrationRunner(db, migrationDir(t))
	if err != nil {
		t.Fatalf("NewMigrationRunner: %v", err)
	}
	ctx := context.Background()
	for {
		executed, err := runner.Down(ctx)
		if err != nil {
			t.Fatalf("Down during reset: %v", err)
		}
		if executed == nil {
			break
		}
	}
	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("Up during reset: %v", err)
	}
}
