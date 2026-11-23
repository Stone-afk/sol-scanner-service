package postgre

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestDiscoverMigrationsReturnsPairedFilesInVersionOrder(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFile(t, dir, "000010_tenth.up.sql", "SELECT 10")
	writeMigrationFile(t, dir, "000010_tenth.down.sql", "SELECT -10")
	writeMigrationFile(t, dir, "000002_second.up.sql", "SELECT 2")
	writeMigrationFile(t, dir, "000002_second.down.sql", "SELECT -2")

	got, err := DiscoverMigrations(dir)
	if err != nil {
		t.Fatalf("DiscoverMigrations() error = %v", err)
	}

	want := []Migration{
		{
			Version:  2,
			Name:     "second",
			UpPath:   filepath.Join(dir, "000002_second.up.sql"),
			DownPath: filepath.Join(dir, "000002_second.down.sql"),
		},
		{
			Version:  10,
			Name:     "tenth",
			UpPath:   filepath.Join(dir, "000010_tenth.up.sql"),
			DownPath: filepath.Join(dir, "000010_tenth.down.sql"),
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverMigrations() = %#v, want %#v", got, want)
	}
}

func TestDiscoverMigrationsRejectsMissingPair(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFile(t, dir, "000001_only_up.up.sql", "SELECT 1")

	_, err := DiscoverMigrations(dir)
	assertDiscoveryError(t, err, "000001", "")
}

func TestDiscoverMigrationsRejectsDuplicateVersion(t *testing.T) {
	dir := t.TempDir()
	writeMigrationFile(t, dir, "000001_first.up.sql", "SELECT 1")
	writeMigrationFile(t, dir, "000001_first.down.sql", "SELECT -1")
	writeMigrationFile(t, dir, "000001_second.up.sql", "SELECT 2")
	writeMigrationFile(t, dir, "000001_second.down.sql", "SELECT -2")

	_, err := DiscoverMigrations(dir)
	assertDiscoveryError(t, err, "000001", "")
}

func TestDiscoverMigrationsRejectsInvalidFilename(t *testing.T) {
	dir := t.TempDir()
	const filename = "1_invalid.up.sql"
	writeMigrationFile(t, dir, filename, "SELECT 1")

	_, err := DiscoverMigrations(dir)
	assertDiscoveryError(t, err, filename, "")
}

func TestDiscoverMigrationsRejectsEmptySQLWithoutLeakingContents(t *testing.T) {
	dir := t.TempDir()
	const secret = "super-secret-migration-value"
	writeMigrationFile(t, dir, "000001_empty.up.sql", " \n\t")
	writeMigrationFile(t, dir, "000001_empty.down.sql", "SELECT '"+secret+"'")

	_, err := DiscoverMigrations(dir)
	assertDiscoveryError(t, err, "000001_empty.up.sql", secret)
}

func TestRepositoryMigrationSet(t *testing.T) {
	dir := filepath.Join(repositoryRoot(t), "scripts", "postgre")
	got, err := DiscoverMigrations(dir)
	if err != nil {
		t.Fatalf("DiscoverMigrations(%q) error = %v", dir, err)
	}
	if len(got) != 2 {
		t.Fatalf("DiscoverMigrations() returned %d migrations, want 2: %#v", len(got), got)
	}
	if got[0].Version != 1 || got[0].Name != "initial_schema" {
		t.Fatalf("migration[0] = version=%d name=%q, want version=1 name=initial_schema", got[0].Version, got[0].Name)
	}
	if got[0].UpPath == "" || got[0].DownPath == "" {
		t.Fatalf("migration[0] paths incomplete: %#v", got[0])
	}
	if filepath.Base(got[0].UpPath) != "000001_initial_schema.up.sql" {
		t.Fatalf("migration[0] UpPath base = %q, want 000001_initial_schema.up.sql", filepath.Base(got[0].UpPath))
	}
	if filepath.Base(got[0].DownPath) != "000001_initial_schema.down.sql" {
		t.Fatalf("migration[0] DownPath base = %q, want 000001_initial_schema.down.sql", filepath.Base(got[0].DownPath))
	}
	if got[1].Version != 2 || got[1].Name != "scan_cursor" {
		t.Fatalf("migration[1] = version=%d name=%q, want version=2 name=scan_cursor", got[1].Version, got[1].Name)
	}
	if got[1].UpPath == "" || got[1].DownPath == "" {
		t.Fatalf("migration[1] paths incomplete: %#v", got[1])
	}
	if filepath.Base(got[1].UpPath) != "000002_scan_cursor.up.sql" {
		t.Fatalf("migration[1] UpPath base = %q, want 000002_scan_cursor.up.sql", filepath.Base(got[1].UpPath))
	}
	if filepath.Base(got[1].DownPath) != "000002_scan_cursor.down.sql" {
		t.Fatalf("migration[1] DownPath base = %q, want 000002_scan_cursor.down.sql", filepath.Base(got[1].DownPath))
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate postgre test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func writeMigrationFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write migration %q: %v", name, err)
	}
}

func assertDiscoveryError(t *testing.T, err error, wantContext, secret string) {
	t.Helper()
	if err == nil {
		t.Fatal("DiscoverMigrations() succeeded, want error")
	}
	if !strings.Contains(err.Error(), wantContext) {
		t.Fatalf("error = %q, want context %q", err, wantContext)
	}
	if secret != "" && strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked migration contents: %q", err)
	}
}
