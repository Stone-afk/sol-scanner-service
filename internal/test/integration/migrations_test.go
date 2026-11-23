//go:build e2e

package integration

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Stone-afk/sol-scanner-service/internal/pkg/postgre"
	"gorm.io/gorm"
)

var businessTables = []string{
	"sol_blocks",
	"sol_transactions",
	"sol_transfers",
	"sol_token_balance_changes",
	"sol_scan_checkpoint",
	"sol_publish_cursor",
	"sol_scan_halts",
}

const scanCursorTable = "sol_scan_cursor"

type columnExpectation struct {
	Name             string
	DataType         string
	NumericPrecision *int
	NumericScale     *int
	Nullable         string // YES | NO
	HasDefault       bool
}

type constraintExpectation struct {
	Type    string // p | u | c
	Columns []string
	Check   string // for CHECK only; substring of pg_get_constraintdef
}

func TestMigrationsUpDownStatus(t *testing.T) {
	db := openTestDB(t)
	dropAllBusinessTables(t, db)
	if err := db.Exec("DROP TABLE IF EXISTS schema_migrations").Error; err != nil {
		t.Fatalf("drop schema_migrations: %v", err)
	}

	runner, err := postgre.NewMigrationRunner(db, migrationDir(t))
	if err != nil {
		t.Fatalf("NewMigrationRunner: %v", err)
	}
	ctx := context.Background()

	status, err := runner.Status(ctx)
	if err != nil {
		t.Fatalf("Status() before Up: %v", err)
	}
	assertMigrationStatus(t, status, false, false)

	executed, err := runner.Up(ctx)
	if err != nil {
		t.Fatalf("Up(): %v", err)
	}
	if len(executed) != 2 ||
		executed[0].Version != 1 || executed[0].Name != "initial_schema" ||
		executed[1].Version != 2 || executed[1].Name != "scan_cursor" {
		t.Fatalf("Up() executed = %#v, want [{1 initial_schema} {2 scan_cursor}]", executed)
	}
	assertInitialSchema(t, db)
	assertScanCursorSchema(t, db)
	assertAppliedMigration(t, db, 1, "initial_schema")
	assertAppliedMigration(t, db, 2, "scan_cursor")

	status, err = runner.Status(ctx)
	if err != nil {
		t.Fatalf("Status() after Up: %v", err)
	}
	assertMigrationStatus(t, status, true, true)

	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("second Up(): %v", err)
	}
	assertAppliedMigration(t, db, 1, "initial_schema")
	assertAppliedMigration(t, db, 2, "scan_cursor")

	downed, err := runner.Down(ctx)
	if err != nil {
		t.Fatalf("Down() version 2: %v", err)
	}
	if downed == nil || downed.Version != 2 || downed.Name != "scan_cursor" {
		t.Fatalf("Down() = %#v, want {2 scan_cursor}", downed)
	}
	assertInitialSchema(t, db)
	assertTableExists(t, db, scanCursorTable, false)
	assertAppliedMigration(t, db, 1, "initial_schema")
	assertAppliedVersionCount(t, db, 2, 0)

	status, err = runner.Status(ctx)
	if err != nil {
		t.Fatalf("Status() after Down version 2: %v", err)
	}
	assertMigrationStatus(t, status, true, false)

	downed, err = runner.Down(ctx)
	if err != nil {
		t.Fatalf("Down() version 1: %v", err)
	}
	if downed == nil || downed.Version != 1 || downed.Name != "initial_schema" {
		t.Fatalf("Down() = %#v, want {1 initial_schema}", downed)
	}
	assertBusinessTablesAbsent(t, db)
	assertTableExists(t, db, scanCursorTable, false)
	assertAppliedVersionCount(t, db, 1, 0)
	assertAppliedVersionCount(t, db, 2, 0)

	status, err = runner.Status(ctx)
	if err != nil {
		t.Fatalf("Status() after Down version 1: %v", err)
	}
	assertMigrationStatus(t, status, false, false)

	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("Up() after Down: %v", err)
	}
	assertInitialSchema(t, db)
	assertScanCursorSchema(t, db)
	assertAppliedMigration(t, db, 1, "initial_schema")
	assertAppliedMigration(t, db, 2, "scan_cursor")
}

func TestSchemaGate(t *testing.T) {
	db := openTestDB(t)
	dropAllBusinessTables(t, db)
	if err := db.Exec("DROP TABLE IF EXISTS schema_migrations").Error; err != nil {
		t.Fatalf("drop schema_migrations: %v", err)
	}

	ctx := context.Background()
	if err := postgre.CheckSchemaVersion(ctx, db, 2); err == nil {
		t.Fatal("CheckSchemaVersion(required=2) succeeded on empty database, want error")
	}

	runner, err := postgre.NewMigrationRunner(db, migrationDir(t))
	if err != nil {
		t.Fatalf("NewMigrationRunner: %v", err)
	}
	if _, err := runner.Up(ctx); err != nil {
		t.Fatalf("Up(): %v", err)
	}
	if err := postgre.CheckSchemaVersion(ctx, db, 1); err != nil {
		t.Fatalf("CheckSchemaVersion(required=1) after Up: %v", err)
	}
	if err := postgre.CheckSchemaVersion(ctx, db, 2); err != nil {
		t.Fatalf("CheckSchemaVersion(required=2) after Up: %v", err)
	}

	if _, err := runner.Down(ctx); err != nil {
		t.Fatalf("Down() version 2: %v", err)
	}
	if err := postgre.CheckSchemaVersion(ctx, db, 2); err == nil {
		t.Fatal("CheckSchemaVersion(required=2) succeeded with only version 1 applied, want error")
	}
	if err := postgre.CheckSchemaVersion(ctx, db, 1); err != nil {
		t.Fatalf("CheckSchemaVersion(required=1) with version 1 applied: %v", err)
	}
}

func assertMigrationStatus(t *testing.T, got []postgre.MigrationStatus, v1Applied, v2Applied bool) {
	t.Helper()
	if len(got) != 2 {
		t.Fatalf("Status() returned %d migrations, want 2: %#v", len(got), got)
	}
	if got[0].Version != 1 || got[0].Name != "initial_schema" || got[0].Applied != v1Applied {
		t.Fatalf("Status()[0] = %#v, want version=1 name=initial_schema applied=%t", got[0], v1Applied)
	}
	if got[1].Version != 2 || got[1].Name != "scan_cursor" || got[1].Applied != v2Applied {
		t.Fatalf("Status()[1] = %#v, want version=2 name=scan_cursor applied=%t", got[1], v2Applied)
	}
}

func assertInitialSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range businessTables {
		assertTableExists(t, db, table, true)
	}
	for table, columns := range expectedColumns() {
		assertColumns(t, db, table, columns)
	}
	for table, constraints := range expectedConstraints() {
		assertConstraints(t, db, table, constraints)
	}
}

func assertScanCursorSchema(t *testing.T, db *gorm.DB) {
	t.Helper()
	assertTableExists(t, db, scanCursorTable, true)
	assertColumns(t, db, scanCursorTable, []columnExpectation{
		{Name: "id", DataType: "smallint", Nullable: "NO"},
		{Name: "handed_slot", DataType: "bigint", Nullable: "NO"},
		{Name: "updated_at", DataType: "bigint", Nullable: "NO"},
	})
	assertConstraints(t, db, scanCursorTable, []constraintExpectation{
		{Type: "c", Columns: []string{"id"}, Check: "id = 1"},
		{Type: "p", Columns: []string{"id"}},
	})
}

func assertBusinessTablesAbsent(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, table := range businessTables {
		assertTableExists(t, db, table, false)
	}
}

func dropAllBusinessTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", scanCursorTable)).Error; err != nil {
		t.Fatalf("drop table %s: %v", scanCursorTable, err)
	}
	for i := len(businessTables) - 1; i >= 0; i-- {
		if err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", businessTables[i])).Error; err != nil {
			t.Fatalf("drop table %s: %v", businessTables[i], err)
		}
	}
}

func assertTableExists(t *testing.T, db *gorm.DB, table string, want bool) {
	t.Helper()
	var exists bool
	if err := db.Raw("SELECT to_regclass(?) IS NOT NULL", "public."+table).Scan(&exists).Error; err != nil {
		t.Fatalf("query table %q existence: %v", table, err)
	}
	if exists != want {
		t.Errorf("table %q exists = %t, want %t", table, exists, want)
	}
}

func assertAppliedMigration(t *testing.T, db *gorm.DB, version int64, name string) {
	t.Helper()
	var got string
	if err := db.Raw("SELECT name FROM schema_migrations WHERE version = ?", version).Scan(&got).Error; err != nil {
		t.Fatalf("read schema_migrations version %d: %v", version, err)
	}
	if got != name {
		t.Fatalf("schema_migrations version %d name = %q, want %q", version, got, name)
	}
}

func assertAppliedVersionCount(t *testing.T, db *gorm.DB, version, want int64) {
	t.Helper()
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&count).Error; err != nil {
		t.Fatalf("count schema_migrations version %d: %v", version, err)
	}
	if count != want {
		t.Errorf("schema_migrations count for version %d = %d, want %d", version, count, want)
	}
}

func assertColumns(t *testing.T, db *gorm.DB, table string, want []columnExpectation) {
	t.Helper()
	type row struct {
		ColumnName             string
		DataType               string
		NumericPrecision       *int
		NumericScale           *int
		IsNullable             string
		ColumnDefault          *string
		CharacterMaximumLength *int
	}
	var rows []row
	if err := db.Raw(`
SELECT column_name, data_type, numeric_precision, numeric_scale, is_nullable, column_default, character_maximum_length
FROM information_schema.columns
WHERE table_schema = current_schema() AND table_name = ?
ORDER BY ordinal_position`, table).Scan(&rows).Error; err != nil {
		t.Fatalf("query columns for %s: %v", table, err)
	}
	if len(rows) != len(want) {
		t.Fatalf("%s column count = %d, want %d\ngot=%#v\nwant=%#v", table, len(rows), len(want), rows, want)
	}
	for i := range want {
		got := rows[i]
		exp := want[i]
		if got.ColumnName != exp.Name || got.DataType != exp.DataType || got.IsNullable != exp.Nullable {
			t.Errorf("%s.%s type/null = (%s,%s), want (%s,%s)", table, exp.Name, got.DataType, got.IsNullable, exp.DataType, exp.Nullable)
		}
		// information_schema 对 bigint/integer/smallint 也会填 numeric_precision/scale；
		// 仅在期望显式给出（或列为 numeric）时断言，避免把“未约束”误当成“必须为 NULL”。
		if exp.DataType == "numeric" || exp.NumericPrecision != nil || exp.NumericScale != nil {
			if !intPtrEqual(got.NumericPrecision, exp.NumericPrecision) || !intPtrEqual(got.NumericScale, exp.NumericScale) {
				t.Errorf("%s.%s numeric = (%v,%v), want (%v,%v)", table, exp.Name, got.NumericPrecision, got.NumericScale, exp.NumericPrecision, exp.NumericScale)
			}
		}
		hasDefault := got.ColumnDefault != nil && strings.TrimSpace(*got.ColumnDefault) != ""
		if hasDefault != exp.HasDefault {
			t.Errorf("%s.%s hasDefault = %t (%v), want %t", table, exp.Name, hasDefault, got.ColumnDefault, exp.HasDefault)
		}
	}
}

func assertConstraints(t *testing.T, db *gorm.DB, table string, want []constraintExpectation) {
	t.Helper()
	type row struct {
		ConType string
		Columns string
		Def     string
	}
	var rows []row
	if err := db.Raw(`
SELECT c.contype AS con_type,
       array_to_string(ARRAY(
           SELECT a.attname
           FROM unnest(c.conkey) WITH ORDINALITY AS u(attnum, ord)
           JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = u.attnum
           ORDER BY u.ord
       ), ',') AS columns,
       pg_get_constraintdef(c.oid) AS def
FROM pg_constraint c
JOIN pg_class r ON r.oid = c.conrelid
JOIN pg_namespace n ON n.oid = r.relnamespace
WHERE n.nspname = current_schema() AND r.relname = ? AND c.contype IN ('p', 'u', 'c')
ORDER BY c.contype, columns`, table).Scan(&rows).Error; err != nil {
		t.Fatalf("query constraints for %s: %v", table, err)
	}

	got := make([]constraintExpectation, 0, len(rows))
	for _, r := range rows {
		cols := []string{}
		if r.Columns != "" {
			cols = strings.Split(r.Columns, ",")
		}
		item := constraintExpectation{Type: r.ConType, Columns: cols}
		if r.ConType == "c" {
			item.Check = r.Def
		}
		got = append(got, item)
	}

	normalizedWant := make([]constraintExpectation, len(want))
	copy(normalizedWant, want)
	sort.Slice(normalizedWant, func(i, j int) bool {
		if normalizedWant[i].Type != normalizedWant[j].Type {
			return normalizedWant[i].Type < normalizedWant[j].Type
		}
		return strings.Join(normalizedWant[i].Columns, ",") < strings.Join(normalizedWant[j].Columns, ",")
	})

	if len(got) != len(normalizedWant) {
		t.Fatalf("%s constraint count = %d, want %d\ngot=%#v\nwant=%#v", table, len(got), len(normalizedWant), got, normalizedWant)
	}
	for i := range normalizedWant {
		exp := normalizedWant[i]
		actual := got[i]
		if actual.Type != exp.Type || !reflect.DeepEqual(actual.Columns, exp.Columns) {
			t.Errorf("%s constraint[%d] = type=%s cols=%v, want type=%s cols=%v", table, i, actual.Type, actual.Columns, exp.Type, exp.Columns)
		}
		if exp.Type == "c" && !strings.Contains(strings.ToLower(actual.Check), strings.ToLower(exp.Check)) {
			t.Errorf("%s CHECK = %q, want substring %q", table, actual.Check, exp.Check)
		}
	}
}

func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func intPtr(v int) *int { return &v }

func expectedColumns() map[string][]columnExpectation {
	return map[string][]columnExpectation{
		"sol_blocks": {
			{Name: "slot", DataType: "bigint", Nullable: "NO"},
			{Name: "blockhash", DataType: "character varying", Nullable: "NO"},
			{Name: "parent_slot", DataType: "bigint", Nullable: "NO"},
			{Name: "previous_blockhash", DataType: "character varying", Nullable: "NO"},
			{Name: "block_height", DataType: "bigint", Nullable: "YES"},
			{Name: "block_time", DataType: "bigint", Nullable: "YES"},
			{Name: "transaction_count", DataType: "integer", Nullable: "NO"},
			{Name: "created_at", DataType: "bigint", Nullable: "NO", HasDefault: true},
		},
		"sol_transactions": {
			{Name: "id", DataType: "bigint", Nullable: "NO", HasDefault: true},
			{Name: "slot", DataType: "bigint", Nullable: "NO"},
			{Name: "tx_index", DataType: "integer", Nullable: "NO"},
			{Name: "signature", DataType: "character varying", Nullable: "NO"},
			{Name: "status", DataType: "smallint", Nullable: "NO"},
			{Name: "version", DataType: "smallint", Nullable: "NO"},
			{Name: "fee", DataType: "bigint", Nullable: "NO"},
			{Name: "fee_payer", DataType: "character varying", Nullable: "NO"},
		},
		"sol_transfers": {
			{Name: "id", DataType: "bigint", Nullable: "NO", HasDefault: true},
			{Name: "slot", DataType: "bigint", Nullable: "NO"},
			{Name: "tx_index", DataType: "integer", Nullable: "NO"},
			{Name: "ix_index", DataType: "smallint", Nullable: "NO"},
			{Name: "inner_index", DataType: "smallint", Nullable: "NO"},
			{Name: "program", DataType: "smallint", Nullable: "NO"},
			{Name: "kind", DataType: "smallint", Nullable: "NO"},
			{Name: "mint", DataType: "character varying", Nullable: "NO", HasDefault: true},
			{Name: "decimals", DataType: "smallint", Nullable: "YES"},
			{Name: "from_account", DataType: "character varying", Nullable: "NO", HasDefault: true},
			{Name: "from_owner", DataType: "character varying", Nullable: "NO", HasDefault: true},
			{Name: "to_account", DataType: "character varying", Nullable: "NO", HasDefault: true},
			{Name: "to_owner", DataType: "character varying", Nullable: "NO", HasDefault: true},
			{Name: "amount", DataType: "numeric", NumericPrecision: intPtr(20), NumericScale: intPtr(0), Nullable: "NO"},
			{Name: "fee", DataType: "numeric", NumericPrecision: intPtr(20), NumericScale: intPtr(0), Nullable: "YES"},
		},
		"sol_token_balance_changes": {
			{Name: "id", DataType: "bigint", Nullable: "NO", HasDefault: true},
			{Name: "slot", DataType: "bigint", Nullable: "NO"},
			{Name: "tx_index", DataType: "integer", Nullable: "NO"},
			{Name: "account_index", DataType: "smallint", Nullable: "NO"},
			{Name: "program", DataType: "smallint", Nullable: "NO"},
			{Name: "account", DataType: "character varying", Nullable: "NO"},
			{Name: "owner", DataType: "character varying", Nullable: "NO", HasDefault: true},
			{Name: "mint", DataType: "character varying", Nullable: "NO"},
			{Name: "decimals", DataType: "smallint", Nullable: "NO"},
			{Name: "pre_amount", DataType: "numeric", NumericPrecision: intPtr(20), NumericScale: intPtr(0), Nullable: "NO"},
			{Name: "post_amount", DataType: "numeric", NumericPrecision: intPtr(20), NumericScale: intPtr(0), Nullable: "NO"},
		},
		"sol_scan_checkpoint": {
			{Name: "id", DataType: "smallint", Nullable: "NO"},
			{Name: "last_block_slot", DataType: "bigint", Nullable: "NO"},
			{Name: "last_blockhash", DataType: "character varying", Nullable: "NO"},
			{Name: "updated_at", DataType: "bigint", Nullable: "NO"},
		},
		"sol_publish_cursor": {
			{Name: "id", DataType: "smallint", Nullable: "NO"},
			{Name: "topic", DataType: "character varying", Nullable: "NO"},
			{Name: "published_slot", DataType: "bigint", Nullable: "NO"},
			{Name: "updated_at", DataType: "bigint", Nullable: "NO"},
		},
		"sol_scan_halts": {
			{Name: "id", DataType: "bigint", Nullable: "NO", HasDefault: true},
			{Name: "reason", DataType: "character varying", Nullable: "NO"},
			{Name: "slot", DataType: "bigint", Nullable: "NO"},
			{Name: "detail", DataType: "text", Nullable: "NO"},
			{Name: "created_at", DataType: "bigint", Nullable: "NO"},
			{Name: "resolved_at", DataType: "bigint", Nullable: "YES"},
		},
	}
}

func expectedConstraints() map[string][]constraintExpectation {
	return map[string][]constraintExpectation{
		"sol_blocks": {
			{Type: "p", Columns: []string{"slot"}},
		},
		"sol_transactions": {
			{Type: "p", Columns: []string{"id"}},
			{Type: "u", Columns: []string{"slot", "tx_index"}},
		},
		"sol_transfers": {
			{Type: "p", Columns: []string{"id"}},
			{Type: "u", Columns: []string{"slot", "tx_index", "ix_index", "inner_index"}},
		},
		"sol_token_balance_changes": {
			{Type: "p", Columns: []string{"id"}},
			{Type: "u", Columns: []string{"slot", "tx_index", "account_index"}},
		},
		"sol_scan_checkpoint": {
			{Type: "c", Columns: []string{"id"}, Check: "id = 1"},
			{Type: "p", Columns: []string{"id"}},
		},
		"sol_publish_cursor": {
			{Type: "c", Columns: []string{"id"}, Check: "id = 1"},
			{Type: "p", Columns: []string{"id"}},
		},
		"sol_scan_halts": {
			{Type: "p", Columns: []string{"id"}},
		},
	}
}
