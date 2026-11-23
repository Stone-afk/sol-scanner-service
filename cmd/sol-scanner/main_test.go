package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Stone-afk/sol-scanner-service/internal/config"
	"github.com/Stone-afk/sol-scanner-service/internal/event/block"
	"github.com/Stone-afk/sol-scanner-service/internal/scanner"
	"github.com/Stone-afk/sol-scanner-service/internal/scanner/provider"
	"github.com/solana-foundation/solana-go/v2/rpc"
)

const (
	testGenesisMainnet = "5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d"
	testGenesisOther   = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
)

func TestRunExitCodes(t *testing.T) {
	// Serial: subtests replace package-level buildRuntime.
	t.Run("AC-071 missing --config", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run(context.Background(), nil, &stdout, &stderr, denyEnv(t))
		if code != 2 {
			t.Fatalf("run() code = %d, want 2; stderr=%q", code, stderr.String())
		}
	})

	t.Run("AC-071 extra positional", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := run(context.Background(), []string{"--config", "x.yaml", "extra"}, &stdout, &stderr, denyEnv(t))
		if code != 2 {
			t.Fatalf("run() code = %d, want 2; stderr=%q", code, stderr.String())
		}
	})

	t.Run("AC-064 startSlot zero no cursor", func(t *testing.T) {
		path := writeScannerConfig(t, testGenesisMainnet, 0, "20ms")
		restore := installFakeRuntime(t, fakeRuntimeOpts{
			genesis:  testGenesisMainnet,
			earliest: 1,
		})
		defer restore()

		if code := runWithLookup(t, path); code != 1 {
			t.Fatalf("run() code = %d, want 1", code)
		}
	})

	t.Run("AC-065 genesis mismatch", func(t *testing.T) {
		path := writeScannerConfig(t, testGenesisMainnet, 500, "20ms")
		restore := installFakeRuntime(t, fakeRuntimeOpts{
			genesis:  testGenesisOther,
			earliest: 1,
		})
		defer restore()

		if code := runWithLookup(t, path); code != 1 {
			t.Fatalf("run() code = %d, want 1", code)
		}
	})

	t.Run("AC-066 startSlot before earliest", func(t *testing.T) {
		path := writeScannerConfig(t, testGenesisMainnet, 500, "20ms")
		restore := installFakeRuntime(t, fakeRuntimeOpts{
			genesis:  testGenesisMainnet,
			earliest: 600,
		})
		defer restore()

		if code := runWithLookup(t, path); code != 1 {
			t.Fatalf("run() code = %d, want 1", code)
		}
	})

	t.Run("AC-069 database error", func(t *testing.T) {
		path := writeScannerConfig(t, testGenesisMainnet, 500, "20ms")

		prev := buildRuntime
		buildRuntime = func(context.Context, config.Config, *slog.Logger) (scannerRuntime, error) {
			return scannerRuntime{}, errors.New("database unavailable")
		}
		defer func() { buildRuntime = prev }()

		if code := runWithLookup(t, path); code != 1 {
			t.Fatalf("run() code = %d, want 1", code)
		}
	})

	t.Run("AC-069 UNSUPPORTED_VERSION", func(t *testing.T) {
		// Scanner process exits 1; CreateHalt is not on the scanner wiring path.
		path := writeScannerConfig(t, testGenesisMainnet, 500, "20ms")
		restore := installFakeRuntime(t, fakeRuntimeOpts{
			genesis:       testGenesisMainnet,
			earliest:      1,
			hasCursor:     true,
			handedSlot:    100,
			finalized:     102,
			unsupportedAt: 101,
		})
		defer restore()

		if code := runWithLookup(t, path); code != 1 {
			t.Fatalf("run() code = %d, want 1", code)
		}
	})

	t.Run("AC-067 AC-068 AC-070 unresolved halt waits then signal exit 0", func(t *testing.T) {
		path := writeScannerConfig(t, testGenesisMainnet, 500, "15ms")
		reader := &fakeReader{
			genesis:   testGenesisMainnet,
			earliest:  1,
			finalized: 1000,
		}
		restore := installFakeRuntime(t, fakeRuntimeOpts{
			reader:     reader,
			hasCursor:  true,
			handedSlot: 100,
			halts:      &haltProbe{unresolved: true},
		})
		defer restore()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan int, 1)
		go func() {
			done <- run(ctx, []string{"--config", path}, io.Discard, io.Discard, testLookupEnv)
		}()

		// Process must stay up while halt is unresolved (no early exit).
		select {
		case code := <-done:
			t.Fatalf("run returned early with code %d before signal", code)
		case <-time.After(40 * time.Millisecond):
		}
		if reader.blockSlotsCalls.Load() != 0 {
			t.Fatalf("BlockSlots called while unresolved halt")
		}

		cancel()
		select {
		case code := <-done:
			if code != 0 {
				t.Fatalf("run() after signal code = %d, want 0", code)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("run did not exit after cancel")
		}
	})
}

func TestFR17ScannerUsesInitX(t *testing.T) {
	// FR-17 / A-026-01: scanner main wires via ioc + ioc/scanner InitX.
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(src)
	required := []string{
		`"github.com/Stone-afk/sol-scanner-service/internal/ioc"`,
		`"github.com/Stone-afk/sol-scanner-service/internal/ioc/scanner"`,
		"ioc.InitDB",
		"InitProvider",
		"InitBlockProducer",
		"InitScanner",
	}
	for _, want := range required {
		if !strings.Contains(text, want) {
			t.Errorf("FR-17: main.go missing %q (must wire via InitX)", want)
		}
	}
}

func TestDepsBanNoParserOrTransactionEvent(t *testing.T) {
	// AC-094 scanner half after A-026-01 ioc split.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	dir := filepath.Dir(file)
	cmd := exec.Command("go", "list", "-deps", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	banned := []string{
		"github.com/Stone-afk/sol-scanner-service/internal/processor/parser",
		"github.com/Stone-afk/sol-scanner-service/internal/event/transaction",
	}
	for _, b := range banned {
		if bytes.Contains(out, []byte(b)) {
			t.Fatalf("forbidden dependency %q present in go list -deps:\n%s", b, out)
		}
	}
}

func runWithLookup(t *testing.T, configPath string) int {
	t.Helper()
	var stdout, stderr bytes.Buffer
	return run(context.Background(), []string{"--config", configPath}, &stdout, &stderr, testLookupEnv)
}

func denyEnv(t *testing.T) func(string) (string, bool) {
	t.Helper()
	return func(string) (string, bool) {
		t.Fatal("argument errors must not read environment")
		return "", false
	}
}

func testLookupEnv(name string) (string, bool) {
	switch name {
	case "POSTGRES_DSN":
		return "postgres://scanner:scanner@127.0.0.1:5432/scanner?sslmode=disable", true
	case "INFURA_API_KEY":
		return "test-infura-key", true
	default:
		return "", false
	}
}

func writeScannerConfig(t *testing.T, genesis string, startSlot uint64, pollInterval string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf(`chain:
  name: solana-mainnet
  genesisHash: %s
  startSlot: %d
rpc:
  httpURL: https://solana-mainnet.infura.io/v3/${INFURA_API_KEY}
scanner:
  pollInterval: %s
  stepSlots: 2
  slotsPerWorker: 2
database:
  dsn: ${POSTGRES_DSN}
kafka:
  brokers: ["127.0.0.1:9092"]
  blockTopic: sol-scanner.blocks
`, genesis, startSlot, pollInterval)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

type fakeRuntimeOpts struct {
	reader        *fakeReader
	genesis       string
	earliest      uint64
	finalized     uint64
	hasCursor     bool
	handedSlot    uint64
	unsupportedAt uint64
	halts         *haltProbe
}

func installFakeRuntime(t *testing.T, opts fakeRuntimeOpts) (restore func()) {
	t.Helper()
	prev := buildRuntime
	buildRuntime = func(_ context.Context, cfg config.Config, logger *slog.Logger) (scannerRuntime, error) {
		reader := opts.reader
		if reader == nil {
			reader = &fakeReader{
				genesis:   opts.genesis,
				earliest:  opts.earliest,
				finalized: opts.finalized,
			}
			if reader.finalized == 0 {
				reader.finalized = 1000
			}
			if opts.unsupportedAt != 0 {
				reader.blockErr = provider.Error{
					Operation: "getBlock",
					Kind:      provider.ErrorKindUnsupportedVersion,
				}
				reader.blockErrSlot = opts.unsupportedAt
				reader.slots = []uint64{opts.unsupportedAt}
			}
		}
		halts := opts.halts
		if halts == nil {
			halts = &haltProbe{}
		}
		cursor := &fakeCursor{slot: opts.handedSlot, ok: opts.hasCursor}
		sc := scanner.New(reader, &fakeProducer{}, cursor, halts, scanner.Config{
			Chain:          cfg.Chain.Name,
			GenesisHash:    cfg.Chain.GenesisHash,
			StartSlot:      cfg.Chain.StartSlot,
			PollInterval:   cfg.Scanner.PollInterval,
			StepSlots:      cfg.Scanner.StepSlots,
			SlotsPerWorker: cfg.Scanner.SlotsPerWorker,
		}, logger)
		return scannerRuntime{
			scanner: sc,
			close:   func() error { return nil },
		}, nil
	}
	return func() { buildRuntime = prev }
}

type haltProbe struct {
	unresolved bool
}

func (h *haltProbe) HasUnresolvedHalt(context.Context) (bool, error) {
	return h.unresolved, nil
}

type fakeCursor struct {
	slot uint64
	ok   bool
}

func (c *fakeCursor) GetHandedSlot(context.Context) (uint64, bool, error) {
	return c.slot, c.ok, nil
}

func (c *fakeCursor) AdvanceHandedSlot(context.Context, uint64, uint64) error { return nil }

func (c *fakeCursor) RewindHandedSlot(context.Context, uint64) error { return nil }

type fakeProducer struct{}

func (*fakeProducer) Produce(context.Context, []block.BlockMessage) error { return nil }

type fakeReader struct {
	genesis         string
	earliest        uint64
	finalized       uint64
	slots           []uint64
	blockErr        error
	blockErrSlot    uint64
	blockSlotsCalls atomic.Uint64
}

func (r *fakeReader) GenesisHash(context.Context) (string, error) { return r.genesis, nil }

func (r *fakeReader) FirstAvailableBlock(context.Context) (uint64, error) { return r.earliest, nil }

func (r *fakeReader) FinalizedSlot(context.Context) (uint64, error) { return r.finalized, nil }

func (r *fakeReader) BlockSlots(context.Context, uint64, uint64) ([]uint64, error) {
	r.blockSlotsCalls.Add(1)
	if r.slots != nil {
		return append([]uint64(nil), r.slots...), nil
	}
	return nil, nil
}

func (r *fakeReader) Block(_ context.Context, slot uint64) (*rpc.GetBlockResult, error) {
	if r.blockErr != nil && (r.blockErrSlot == 0 || r.blockErrSlot == slot) {
		return nil, r.blockErr
	}
	return &rpc.GetBlockResult{}, nil
}
