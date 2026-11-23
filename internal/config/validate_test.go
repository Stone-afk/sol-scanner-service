package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	// AC-007, AC-079, CFG-05. Errors must name the field path and must not carry URL or DSN values.
	tests := []struct {
		name          string
		mutate        func(string) string
		scannerOnly   bool
		processorOnly bool
		wantOK        bool
		wantStart     uint64
		wantStep      uint64
		wantPerWorker uint64
		wantErrPart   string
		forbid        []string
	}{
		{
			name:          "AC-079 stepSlots and slotsPerWorker",
			mutate:        func(base string) string { return base },
			wantOK:        true,
			wantStart:     471991643,
			wantStep:      20000,
			wantPerWorker: 500,
		},
		{
			name: "AC-079 indivisible step",
			mutate: func(base string) string {
				return strings.Replace(base, "stepSlots: 20000", "stepSlots: 20001", 1)
			},
			wantOK:        true,
			wantStart:     471991643,
			wantStep:      20001,
			wantPerWorker: 500,
		},
		{
			name: "step bounds inclusive",
			mutate: func(base string) string {
				base = strings.Replace(base, "stepSlots: 20000", "stepSlots: 500000", 1)
				return strings.Replace(base, "slotsPerWorker: 500", "slotsPerWorker: 500000", 1)
			},
			wantOK:        true,
			wantStart:     471991643,
			wantStep:      500000,
			wantPerWorker: 500000,
		},
		{
			name: "step lower bounds",
			mutate: func(base string) string {
				base = strings.Replace(base, "stepSlots: 20000", "stepSlots: 1", 1)
				return strings.Replace(base, "slotsPerWorker: 500", "slotsPerWorker: 1", 1)
			},
			wantOK:        true,
			wantStart:     471991643,
			wantStep:      1,
			wantPerWorker: 1,
		},
		{
			name: "AC-079 stepSlots zero",
			mutate: func(base string) string {
				return strings.Replace(base, "stepSlots: 20000", "stepSlots: 0", 1)
			},
			wantErrPart: "scanner.stepSlots",
		},
		{
			name: "AC-079 stepSlots above 500000",
			mutate: func(base string) string {
				return strings.Replace(base, "stepSlots: 20000", "stepSlots: 500001", 1)
			},
			wantErrPart: "scanner.stepSlots",
		},
		{
			name: "AC-079 slotsPerWorker zero",
			mutate: func(base string) string {
				return strings.Replace(base, "slotsPerWorker: 500", "slotsPerWorker: 0", 1)
			},
			wantErrPart: "scanner.slotsPerWorker",
		},
		{
			name: "slotsPerWorker above 500000",
			mutate: func(base string) string {
				return strings.Replace(base, "slotsPerWorker: 500", "slotsPerWorker: 500001", 1)
			},
			wantErrPart: "scanner.slotsPerWorker",
		},
		{
			name: "AC-007 unknown scanWindowSlots",
			mutate: func(base string) string {
				return strings.Replace(base, "pollInterval: 500ms\n", "pollInterval: 500ms\n  scanWindowSlots: 100\n", 1)
			},
			wantErrPart: "scanWindowSlots",
		},
		{
			name: "AC-007 unknown fetchConcurrency",
			mutate: func(base string) string {
				return strings.Replace(base, "pollInterval: 500ms\n", "pollInterval: 500ms\n  fetchConcurrency: 8\n", 1)
			},
			wantErrPart: "fetchConcurrency",
		},
		{
			name: "AC-007 omit startSlot",
			mutate: func(base string) string {
				return strings.Replace(base, "  startSlot: 471991643\n", "", 1)
			},
			wantOK:        true,
			wantStart:     0,
			wantStep:      20000,
			wantPerWorker: 500,
		},
		{
			name: "empty chain.name",
			mutate: func(base string) string {
				return strings.Replace(base, "name: solana-mainnet", "name: \"\"", 1)
			},
			wantErrPart: "chain.name",
		},
		{
			name: "invalid genesisHash",
			mutate: func(base string) string {
				return strings.Replace(base,
					"genesisHash: 5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d",
					"genesisHash: \"0OIl\"", 1)
			},
			wantErrPart: "chain.genesisHash",
			forbid:      []string{"0OIl"},
		},
		{
			name: "CFG-05 non-http URL",
			mutate: func(base string) string {
				return strings.Replace(base,
					"httpURL: https://solana-mainnet.infura.io/v3/${INFURA_API_KEY}",
					"httpURL: wss://h/x", 1)
			},
			scannerOnly: true,
			wantErrPart: "rpc.httpURL",
			forbid:      []string{"wss://h/x"},
		},
		{
			name: "rpc.httpURL missing host",
			mutate: func(base string) string {
				return strings.Replace(base,
					"httpURL: https://solana-mainnet.infura.io/v3/${INFURA_API_KEY}",
					"httpURL: https:///x", 1)
			},
			scannerOnly: true,
			wantErrPart: "rpc.httpURL",
			forbid:      []string{"https:///x"},
		},
		{
			name: "pollInterval zero",
			mutate: func(base string) string {
				return strings.Replace(base, "pollInterval: 500ms", "pollInterval: 0s", 1)
			},
			wantErrPart: "scanner.pollInterval",
		},
		{
			name: "empty brokers",
			mutate: func(base string) string {
				return strings.Replace(base, `brokers: ["127.0.0.1:9092"]`, "brokers: []", 1)
			},
			wantErrPart: "kafka.brokers",
		},
		{
			name: "CFG-05 transactionTopic space",
			mutate: func(base string) string {
				return strings.Replace(base, "transactionTopic: sol-scanner.transactions", `transactionTopic: "a b"`, 1)
			},
			wantErrPart: "kafka.transactionTopic",
			forbid:      []string{"a b"},
		},
		{
			name: "CFG-05 blockTopic space",
			mutate: func(base string) string {
				return strings.Replace(base, "blockTopic: sol-scanner.blocks", `blockTopic: "bad topic"`, 1)
			},
			wantErrPart: "kafka.blockTopic",
			forbid:      []string{"bad topic"},
		},
		{
			name: "blockTopic length 249",
			mutate: func(base string) string {
				return strings.Replace(base, "blockTopic: sol-scanner.blocks", "blockTopic: "+strings.Repeat("b", 249), 1)
			},
			wantOK:        true,
			wantStart:     471991643,
			wantStep:      20000,
			wantPerWorker: 500,
		},
		{
			name: "blockTopic length 250",
			mutate: func(base string) string {
				return strings.Replace(base, "blockTopic: sol-scanner.blocks", "blockTopic: "+strings.Repeat("c", 250), 1)
			},
			wantErrPart: "kafka.blockTopic",
		},
		{
			name: "scanner allows empty transactionTopic",
			mutate: func(base string) string {
				return strings.Replace(base, "  transactionTopic: sol-scanner.transactions\n", "", 1)
			},
			scannerOnly:   true,
			wantOK:        true,
			wantStart:     471991643,
			wantStep:      20000,
			wantPerWorker: 500,
		},
		{
			name: "processor requires transactionTopic",
			mutate: func(base string) string {
				return strings.Replace(base, "  transactionTopic: sol-scanner.transactions\n", "", 1)
			},
			processorOnly: true,
			wantErrPart:   "kafka.transactionTopic",
		},
		{
			name: "negative startSlot",
			mutate: func(base string) string {
				return strings.Replace(base, "startSlot: 471991643", "startSlot: -1", 1)
			},
			wantErrPart: "invalid YAML configuration",
		},
	}

	loaders := []struct {
		name    string
		scanner bool
		load    func(string) (uint64, uint64, uint64, error)
	}{
		{
			name:    "scanner",
			scanner: true,
			load: func(path string) (uint64, uint64, uint64, error) {
				cfg, err := LoadForScanner(path, testLookupEnv)
				if err != nil {
					return 0, 0, 0, err
				}
				return cfg.Chain.StartSlot, cfg.Scanner.StepSlots, cfg.Scanner.SlotsPerWorker, nil
			},
		},
		{
			name: "processor",
			load: func(path string) (uint64, uint64, uint64, error) {
				cfg, err := LoadForProcessor(path, testLookupEnv)
				if err != nil {
					return 0, 0, 0, err
				}
				return cfg.Chain.StartSlot, cfg.Scanner.StepSlots, cfg.Scanner.SlotsPerWorker, nil
			},
		},
	}

	for _, tt := range tests {
		for _, loader := range loaders {
			if tt.scannerOnly && !loader.scanner {
				continue
			}
			if tt.processorOnly && loader.scanner {
				continue
			}
			t.Run(tt.name+"/"+loader.name, func(t *testing.T) {
				path := writeConfig(t, tt.mutate(validConfigYAML()))
				start, step, perWorker, err := loader.load(path)
				if tt.wantOK {
					if err != nil {
						requireNoSecrets(t, err.Error())
						t.Fatal("load failed, want success")
					}
					if start != tt.wantStart || step != tt.wantStep || perWorker != tt.wantPerWorker {
						t.Errorf("got start=%d step=%d perWorker=%d, want %d/%d/%d",
							start, step, perWorker, tt.wantStart, tt.wantStep, tt.wantPerWorker)
					}
					if loader.scanner {
						cfg, loadErr := LoadForScanner(path, testLookupEnv)
						if loadErr != nil {
							requireNoSecrets(t, loadErr.Error())
							t.Fatal("reload failed, want success")
						}
						if cfg.Scanner.PollInterval != 500*time.Millisecond {
							t.Errorf("PollInterval = %v, want 500ms", cfg.Scanner.PollInterval)
						}
					}
					return
				}
				if err == nil {
					t.Fatal("load succeeded, want error")
				}
				msg := err.Error()
				requireNoSecrets(t, msg)
				for _, part := range tt.forbid {
					if strings.Contains(msg, part) {
						t.Fatalf("error contains forbidden configuration value")
					}
				}
				if !strings.Contains(msg, tt.wantErrPart) {
					t.Errorf("error = %q, want substring %q", msg, tt.wantErrPart)
				}
			})
		}
	}
}
