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

	t.Run("AC-069 database error", func(t *testing.T) {
		path := writeProcessorConfig(t, "20ms")
		prev := buildRuntime
		buildRuntime = func(context.Context, config.ProcessorConfig, *slog.Logger) (processorRuntime, error) {
			return processorRuntime{}, errors.New("database unavailable")
		}
		defer func() { buildRuntime = prev }()

		if code := runWithLookup(t, path); code != 1 {
			t.Fatalf("run() code = %d, want 1", code)
		}
	})

	t.Run("AC-067 AC-068 AC-070 unresolved halt starts publish not apply then signal exit 0", func(t *testing.T) {
		path := writeProcessorConfig(t, "15ms")
		var applyStarts, publishStarts atomic.Uint64
		prev := buildRuntime
		buildRuntime = func(_ context.Context, _ config.ProcessorConfig, _ *slog.Logger) (processorRuntime, error) {
			return processorRuntime{
				hasUnresolvedHalt: func(context.Context) (bool, error) { return true, nil },
				startApply: func(ctx context.Context) error {
					applyStarts.Add(1)
					<-ctx.Done()
					return ctx.Err()
				},
				startPublish: func(ctx context.Context) error {
					publishStarts.Add(1)
					<-ctx.Done()
					return ctx.Err()
				},
				close: func() error { return nil },
			}, nil
		}
		defer func() { buildRuntime = prev }()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan int, 1)
		go func() {
			done <- run(ctx, []string{"--config", path}, io.Discard, io.Discard, processorLookupEnv)
		}()

		deadline := time.Now().Add(2 * time.Second)
		for publishStarts.Load() == 0 {
			if time.Now().After(deadline) {
				t.Fatal("publish loop did not start while halt unresolved")
			}
			time.Sleep(5 * time.Millisecond)
		}

		select {
		case code := <-done:
			t.Fatalf("run returned early with code %d before signal", code)
		case <-time.After(40 * time.Millisecond):
		}
		if applyStarts.Load() != 0 {
			t.Fatalf("apply started %d times while unresolved halt", applyStarts.Load())
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
		if applyStarts.Load() != 0 {
			t.Fatalf("apply started after cancel; starts=%d", applyStarts.Load())
		}
		if publishStarts.Load() == 0 {
			t.Fatal("publish loop never started")
		}
	})

	t.Run("AC-078 LoadForProcessor without INFURA", func(t *testing.T) {
		path := writeProcessorConfig(t, "20ms")
		prev := buildRuntime
		buildRuntime = func(_ context.Context, cfg config.ProcessorConfig, _ *slog.Logger) (processorRuntime, error) {
			if cfg.Database.DSN == "" {
				t.Fatal("expected expanded database dsn")
			}
			return processorRuntime{
				hasUnresolvedHalt: func(context.Context) (bool, error) { return false, nil },
				startApply:        func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
				startPublish:      func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
				close:             func() error { return nil },
			}, nil
		}
		defer func() { buildRuntime = prev }()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan int, 1)
		go func() {
			done <- run(ctx, []string{"--config", path}, io.Discard, io.Discard, processorLookupEnvNoInfura)
		}()
		time.Sleep(20 * time.Millisecond)
		cancel()
		select {
		case code := <-done:
			if code != 0 {
				t.Fatalf("run() code = %d, want 0; INFURA must not be required", code)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("run did not exit after cancel")
		}
	})
}

func TestFR17ProcessorUsesInitXNotLeafWiring(t *testing.T) {
	// FR-17 / A-026-01: production main must call shared + processor InitX.
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	text := string(src)
	required := []string{
		`"github.com/Stone-afk/sol-scanner-service/internal/ioc"`,
		`"github.com/Stone-afk/sol-scanner-service/internal/ioc/processor"`,
		"ioc.InitDB",
		"ioc.CloseDB",
		"InitBlockConsumer",
		"InitProcessor",
		"InitTransactionProducer",
		"HasUnresolvedHalt",
	}
	for _, want := range required {
		if !strings.Contains(text, want) {
			t.Errorf("FR-17: main.go missing %q (must wire via InitX)", want)
		}
	}
	forbidden := []string{
		"postgre.Open",
		"postgre.Ping",
		"postgre.CheckSchemaVersion",
		"kafka.NewConsumer",
		"kafka.NewProducer",
		"repository.NewScanRepository",
		"repository.NewPublishRepository",
		"processor.New(",
		"block.NewConsumer",
		"transaction.NewProducer",
		"alert.NewLogAlerter",
	}
	for _, bad := range forbidden {
		if strings.Contains(text, bad) {
			t.Errorf("FR-17: main.go must not leaf-wire %q; use ioc InitX", bad)
		}
	}
}

func TestDepsBanNoProviderOrChainAPI(t *testing.T) {
	// AC-094 processor half + parent ioc isolation after A-026-01 split.
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
		"github.com/Stone-afk/sol-scanner-service/internal/scanner/provider",
		"github.com/Stone-afk/chain-api-go",
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
	return run(context.Background(), []string{"--config", configPath}, &stdout, &stderr, processorLookupEnv)
}

func denyEnv(t *testing.T) func(string) (string, bool) {
	t.Helper()
	return func(string) (string, bool) {
		t.Fatal("argument errors must not read environment")
		return "", false
	}
}

func processorLookupEnv(name string) (string, bool) {
	switch name {
	case "POSTGRES_DSN":
		return "postgres://scanner:scanner@127.0.0.1:5432/scanner?sslmode=disable", true
	default:
		return "", false
	}
}

func processorLookupEnvNoInfura(name string) (string, bool) {
	if name == "INFURA_API_KEY" {
		return "", false
	}
	return processorLookupEnv(name)
}

func writeProcessorConfig(t *testing.T, pollInterval string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf(`chain:
  name: solana-mainnet
  genesisHash: 5eykt4UsFv8P8NJdTREpY1vzqKqZKvdpKuc147dw2N9d
  startSlot: 500
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
  transactionTopic: sol-scanner.transactions
`, pollInterval)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
