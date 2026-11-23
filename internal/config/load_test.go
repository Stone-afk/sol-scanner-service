package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	testDSN    = "postgres://tester:dsn-secret@db.example.test:5432/sol_scanner"
	testAPIKey = "infura-api-key-secret"
	testURL    = "https://solana-mainnet.infura.io/v3/" + testAPIKey
)

func TestLoadForScanner(t *testing.T) {
	// CFG-01 on the v1.6 shape, plus AC-006 rejection cases. The on-disk example is separate.
	t.Run("CFG-01", func(t *testing.T) {
		path := writeConfig(t, validConfigYAML())
		cfg, err := LoadForScanner(path, testLookupEnv)
		if err != nil {
			requireNoSecrets(t, err.Error())
			t.Fatalf("LoadForScanner() error = %v", err)
		}
		assertScannerShape(t, cfg)
	})

	rejects := []struct {
		name        string
		mutate      func(string) string
		lookup      func(string) (string, bool)
		wantErrPart string
	}{
		{
			name: "AC-006 unknown field",
			mutate: func(base string) string {
				return strings.Replace(base, "scanner:\n", "scanner:\n  foo: 1\n", 1)
			},
			lookup:      testLookupEnv,
			wantErrPart: "foo",
		},
		{
			name: "AC-006 missing POSTGRES_DSN",
			mutate: func(base string) string {
				return base
			},
			lookup: func(name string) (string, bool) {
				if name == "INFURA_API_KEY" {
					return testAPIKey, true
				}
				return "", false
			},
			wantErrPart: `"POSTGRES_DSN"`,
		},
		{
			name: "AC-006 unresolved placeholder",
			mutate: func(base string) string {
				return strings.Replace(base, "dsn: ${POSTGRES_DSN}", "dsn: ${}", 1)
			},
			lookup:      testLookupEnv,
			wantErrPart: "unresolved environment placeholder",
		},
		{
			name: "AC-006 second YAML document",
			mutate: func(base string) string {
				return base + "---\ndatabase:\n  dsn: ignored\n"
			},
			lookup:      testLookupEnv,
			wantErrPart: "invalid YAML configuration",
		},
	}
	for _, tt := range rejects {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.mutate(validConfigYAML()))
			_, err := LoadForScanner(path, tt.lookup)
			if err == nil {
				t.Fatal("LoadForScanner() succeeded, want error")
			}
			requireNoSecrets(t, err.Error())
			if !strings.Contains(err.Error(), tt.wantErrPart) {
				t.Errorf("error = %q, want substring %q", err.Error(), tt.wantErrPart)
			}
		})
	}
}

func TestLoadForProcessor(t *testing.T) {
	// CFG-02: the same v1.6 input without INFURA_API_KEY still loads, and carries no RPC endpoint.
	t.Run("CFG-02", func(t *testing.T) {
		path := writeConfig(t, validConfigYAML())
		proc, err := LoadForProcessor(path, lookupWithoutInfura)
		if err != nil {
			requireNoSecrets(t, err.Error())
			t.Fatalf("LoadForProcessor() error = %v", err)
		}
		assertProcessorShape(t, proc)
		assertNoRPCMaterial(t, proc, nil)
	})

	t.Run("AC-078 ignores rpc.httpURL", func(t *testing.T) {
		raw := strings.Replace(validConfigYAML(),
			"httpURL: https://solana-mainnet.infura.io/v3/${INFURA_API_KEY}",
			"httpURL: wss://rpc.example.test/not-used", 1)
		path := writeConfig(t, raw)
		proc, err := LoadForProcessor(path, lookupWithoutInfura)
		if err != nil {
			requireNoSecrets(t, err.Error())
			t.Fatalf("LoadForProcessor() error = %v", err)
		}
		assertNoRPCMaterial(t, proc, []string{"wss://", "rpc.example.test", "not-used"})
	})

	t.Run("AC-006 unknown field", func(t *testing.T) {
		raw := strings.Replace(validConfigYAML(), "scanner:\n", "scanner:\n  foo: 1\n", 1)
		path := writeConfig(t, raw)
		_, err := LoadForProcessor(path, lookupWithoutInfura)
		if err == nil {
			t.Fatal("LoadForProcessor() succeeded, want error")
		}
		requireNoSecrets(t, err.Error())
		if !strings.Contains(err.Error(), "foo") {
			t.Errorf("error = %q, want substring %q", err.Error(), "foo")
		}
	})

	t.Run("rejects unknown rpc field", func(t *testing.T) {
		raw := strings.Replace(validConfigYAML(),
			"httpURL: https://solana-mainnet.infura.io/v3/${INFURA_API_KEY}\n",
			"httpURL: https://solana-mainnet.infura.io/v3/${INFURA_API_KEY}\n  typo: true\n", 1)
		path := writeConfig(t, raw)
		_, err := LoadForProcessor(path, lookupWithoutInfura)
		if err == nil {
			t.Fatal("LoadForProcessor() succeeded, want unknown rpc field error")
		}
		requireNoSecrets(t, err.Error())
		if !strings.Contains(err.Error(), "rpc.") || !strings.Contains(err.Error(), "typo") {
			t.Errorf("error = %q, want rpc field path", err.Error())
		}
	})

	t.Run("omitted rpc section", func(t *testing.T) {
		raw := strings.Replace(validConfigYAML(),
			"rpc:\n  httpURL: https://solana-mainnet.infura.io/v3/${INFURA_API_KEY}\n", "", 1)
		path := writeConfig(t, raw)
		proc, err := LoadForProcessor(path, lookupWithoutInfura)
		if err != nil {
			requireNoSecrets(t, err.Error())
			t.Fatal("LoadForProcessor() failed, want success without rpc")
		}
		assertProcessorShape(t, proc)
		assertNoRPCMaterial(t, proc, nil)
	})
}

func TestLoadForScannerExampleFile(t *testing.T) {
	// CFG-01 against the committed example. Stays red until that file uses the v1.6 keys.
	path := filepath.Join("..", "..", "config", "config.example.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(config.example.yaml) error = %v", err)
	}
	contents := string(raw)
	for _, token := range []string{
		"${INFURA_API_KEY}",
		"dsn: ${POSTGRES_DSN}",
		"stepSlots: 20000",
		"slotsPerWorker: 500",
		"blockTopic: sol-scanner.blocks",
		"transactionTopic: sol-scanner.transactions",
	} {
		if !strings.Contains(contents, token) {
			t.Errorf("config.example.yaml missing %s", token)
		}
	}
	for _, old := range []string{"scanWindowSlots", "fetchConcurrency"} {
		if strings.Contains(contents, old) {
			t.Errorf("config.example.yaml still contains %s", old)
		}
	}

	cfg, err := LoadForScanner(path, func(name string) (string, bool) {
		switch name {
		case "INFURA_API_KEY":
			return "k", true
		case "POSTGRES_DSN":
			return "d", true
		default:
			return "", false
		}
	})
	if err != nil {
		requireNoSecrets(t, err.Error())
		t.Fatalf("LoadForScanner(config.example.yaml) error = %v", err)
	}
	if cfg.Scanner.StepSlots != 20000 || cfg.Scanner.SlotsPerWorker != 500 {
		t.Errorf("example step = %d/%d, want 20000/500", cfg.Scanner.StepSlots, cfg.Scanner.SlotsPerWorker)
	}
	if cfg.Kafka.BlockTopic != "sol-scanner.blocks" {
		t.Errorf("example blockTopic = %q", cfg.Kafka.BlockTopic)
	}
	if cfg.Kafka.TransactionTopic != "sol-scanner.transactions" {
		t.Errorf("example transactionTopic = %q", cfg.Kafka.TransactionTopic)
	}
	if !strings.HasSuffix(cfg.RPC.HTTPURL, "/v3/k") {
		t.Error("example RPC URL was not expanded from INFURA_API_KEY")
	}
	if cfg.Database.DSN != "d" {
		t.Error("example DSN was not expanded from POSTGRES_DSN")
	}

	proc, err := LoadForProcessor(path, func(name string) (string, bool) {
		if name == "POSTGRES_DSN" {
			return "d", true
		}
		return "", false
	})
	if err != nil {
		requireNoSecrets(t, err.Error())
		t.Fatalf("LoadForProcessor(config.example.yaml) error = %v", err)
	}
	assertNoRPCMaterial(t, proc, []string{"infura", "https://", "/v3/k"})
	if proc.Database.DSN != "d" {
		t.Error("processor example DSN was not expanded from POSTGRES_DSN")
	}
}

func TestLoadForMigration(t *testing.T) {
	// CFG-04: only POSTGRES_DSN is set. RPC and other sections keep their placeholders.
	path := writeConfig(t, validConfigYAML())
	database, err := LoadForMigration(path, func(name string) (string, bool) {
		if name == "POSTGRES_DSN" {
			return testDSN, true
		}
		return "", false
	})
	if err != nil {
		requireNoSecrets(t, err.Error())
		t.Fatalf("LoadForMigration() error = %v", err)
	}
	if database.DSN != testDSN {
		t.Fatal("migration DSN mismatch")
	}
}

func TestLoadForScannerRequiresInfura(t *testing.T) {
	// CFG-02 scanner half of the same input used by TestLoadForProcessor.
	path := writeConfig(t, validConfigYAML())
	_, err := LoadForScanner(path, lookupWithoutInfura)
	if err == nil {
		t.Fatal("LoadForScanner() succeeded, want missing INFURA_API_KEY")
	}
	requireNoSecrets(t, err.Error())
	if !strings.Contains(err.Error(), `"INFURA_API_KEY"`) {
		t.Errorf("error = %q, want variable name", err.Error())
	}
}

func validConfigYAML() string {
	return `chain:
  name: solana-mainnet
  genesisHash: 5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d
  startSlot: 471991643
rpc:
  httpURL: https://solana-mainnet.infura.io/v3/${INFURA_API_KEY}
scanner:
  pollInterval: 500ms
  stepSlots: 20000
  slotsPerWorker: 500
database:
  dsn: ${POSTGRES_DSN}
kafka:
  brokers: ["127.0.0.1:9092"]
  blockTopic: sol-scanner.blocks
  transactionTopic: sol-scanner.transactions
`
}

func testLookupEnv(name string) (string, bool) {
	switch name {
	case "POSTGRES_DSN":
		return testDSN, true
	case "INFURA_API_KEY":
		return testAPIKey, true
	default:
		return "", false
	}
}

func lookupWithoutInfura(name string) (string, bool) {
	if name == "POSTGRES_DSN" {
		return testDSN, true
	}
	return "", false
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func assertScannerShape(t *testing.T, cfg Config) {
	t.Helper()
	if cfg.Chain.Name != "solana-mainnet" {
		t.Errorf("Chain.Name = %q, want solana-mainnet", cfg.Chain.Name)
	}
	if cfg.Chain.StartSlot != 471991643 {
		t.Errorf("StartSlot = %d, want 471991643", cfg.Chain.StartSlot)
	}
	if cfg.Scanner.PollInterval != 500*time.Millisecond {
		t.Errorf("PollInterval = %v, want 500ms", cfg.Scanner.PollInterval)
	}
	if cfg.Scanner.StepSlots != 20000 {
		t.Errorf("StepSlots = %d, want 20000", cfg.Scanner.StepSlots)
	}
	if cfg.Scanner.SlotsPerWorker != 500 {
		t.Errorf("SlotsPerWorker = %d, want 500", cfg.Scanner.SlotsPerWorker)
	}
	if cfg.Kafka.BlockTopic != "sol-scanner.blocks" {
		t.Errorf("BlockTopic = %q, want sol-scanner.blocks", cfg.Kafka.BlockTopic)
	}
	if cfg.Kafka.TransactionTopic != "sol-scanner.transactions" {
		t.Errorf("TransactionTopic = %q, want sol-scanner.transactions", cfg.Kafka.TransactionTopic)
	}
	if len(cfg.Kafka.Brokers) != 1 || cfg.Kafka.Brokers[0] != "127.0.0.1:9092" {
		t.Errorf("Brokers = %#v, want [127.0.0.1:9092]", cfg.Kafka.Brokers)
	}
	if !strings.HasSuffix(cfg.RPC.HTTPURL, "/"+testAPIKey) {
		t.Error("RPC URL was not expanded from INFURA_API_KEY")
	}
	if strings.Contains(cfg.RPC.HTTPURL, "${") {
		t.Error("RPC URL still contains a placeholder")
	}
	if cfg.Database.DSN != testDSN {
		t.Fatal("DSN mismatch")
	}
}

func assertProcessorShape(t *testing.T, cfg ProcessorConfig) {
	t.Helper()
	if cfg.Chain.Name != "solana-mainnet" {
		t.Errorf("Chain.Name = %q, want solana-mainnet", cfg.Chain.Name)
	}
	if cfg.Chain.GenesisHash != "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d" {
		t.Error("GenesisHash mismatch")
	}
	if cfg.Chain.StartSlot != 471991643 {
		t.Errorf("StartSlot = %d, want 471991643", cfg.Chain.StartSlot)
	}
	if cfg.Scanner.PollInterval != 500*time.Millisecond {
		t.Errorf("PollInterval = %v, want 500ms", cfg.Scanner.PollInterval)
	}
	if cfg.Scanner.StepSlots != 20000 || cfg.Scanner.SlotsPerWorker != 500 {
		t.Errorf("step = %d/%d, want 20000/500", cfg.Scanner.StepSlots, cfg.Scanner.SlotsPerWorker)
	}
	if cfg.Kafka.BlockTopic != "sol-scanner.blocks" {
		t.Errorf("BlockTopic = %q", cfg.Kafka.BlockTopic)
	}
	if cfg.Kafka.TransactionTopic != "sol-scanner.transactions" {
		t.Errorf("TransactionTopic = %q", cfg.Kafka.TransactionTopic)
	}
	if len(cfg.Kafka.Brokers) != 1 || cfg.Kafka.Brokers[0] != "127.0.0.1:9092" {
		t.Errorf("Brokers = %#v, want [127.0.0.1:9092]", cfg.Kafka.Brokers)
	}
	if cfg.Database.DSN != testDSN {
		t.Fatal("DSN mismatch")
	}
}

func assertNoRPCMaterial(t *testing.T, cfg any, extra []string) {
	t.Helper()
	v := reflect.ValueOf(cfg)
	if v.Kind() != reflect.Struct {
		t.Fatalf("processor config kind = %s, want struct", v.Kind())
	}
	if _, ok := v.Type().FieldByName("RPC"); ok {
		t.Fatal("processor config provides RPC client configuration")
	}
	forbidden := append([]string{"https://", "http://", "infura.io", testAPIKey, testURL}, extra...)
	var walk func(reflect.Value)
	walk = func(val reflect.Value) {
		if !val.IsValid() {
			return
		}
		switch val.Kind() {
		case reflect.String:
			s := val.String()
			for _, part := range forbidden {
				if part != "" && strings.Contains(s, part) {
					t.Fatal("processor config contains RPC endpoint material")
				}
			}
		case reflect.Struct:
			for i := 0; i < val.NumField(); i++ {
				walk(val.Field(i))
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < val.Len(); i++ {
				walk(val.Index(i))
			}
		case reflect.Pointer, reflect.Interface:
			if !val.IsNil() {
				walk(val.Elem())
			}
		}
	}
	walk(v)
}

func requireNoSecrets(t *testing.T, msg string) {
	t.Helper()
	for _, secret := range []string{testDSN, testAPIKey, testURL, "https://solana-mainnet.infura.io", "postgres://"} {
		if strings.Contains(msg, secret) {
			t.Fatalf("output contains forbidden configuration value")
		}
	}
}
