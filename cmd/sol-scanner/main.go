package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Stone-afk/sol-scanner-service/internal/config"
	"github.com/Stone-afk/sol-scanner-service/internal/ioc"
	iocscanner "github.com/Stone-afk/sol-scanner-service/internal/ioc/scanner"
	"github.com/Stone-afk/sol-scanner-service/internal/scanner"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv))
}

// scannerRuntime holds the assembled scanner and resource closer.
type scannerRuntime struct {
	scanner *scanner.Scanner
	close   func() error
}

// buildRuntime assembles production dependencies. Tests replace this to inject
// a fake Reader and avoid mainnet or real Kafka/PostgreSQL.
var buildRuntime = defaultBuildRuntime

func defaultBuildRuntime(ctx context.Context, cfg config.Config, logger *slog.Logger) (scannerRuntime, error) {
	db, err := ioc.InitDB(ctx, cfg.Database)
	if err != nil {
		return scannerRuntime{}, err
	}
	reader, err := iocscanner.InitProvider(cfg.RPC)
	if err != nil {
		_ = ioc.CloseDB(db)
		return scannerRuntime{}, err
	}
	producer, err := iocscanner.InitBlockProducer(cfg.Kafka)
	if err != nil {
		_ = ioc.CloseDB(db)
		return scannerRuntime{}, err
	}
	sc := iocscanner.InitScanner(reader, producer, db, cfg, logger)
	return scannerRuntime{
		scanner: sc,
		close: func() error {
			producer.Close()
			return ioc.CloseDB(db)
		},
	}, nil
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	flags := flag.NewFlagSet("sol-scanner", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "configuration file")
	if err := flags.Parse(args); err != nil {
		fmt.Fprintln(stderr, "invalid arguments")
		return 2
	}
	if *configPath == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "--config is required and positional arguments are not allowed")
		return 2
	}

	cfg, err := config.LoadForScanner(*configPath, lookupEnv)
	if err != nil {
		fmt.Fprintf(stderr, "load configuration: %v\n", err)
		return 1
	}

	logger := slog.New(slog.NewTextHandler(stdout, nil)).With("command", "sol-scanner")
	rt, err := buildRuntime(ctx, cfg, logger)
	if err != nil {
		fmt.Fprintf(stderr, "initialize scanner: %v\n", err)
		return 1
	}

	runErr := func() error {
		if err := rt.scanner.Preflight(ctx); err != nil {
			return err
		}
		return rt.scanner.Run(ctx)
	}()
	closeErr := rt.close()

	if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
		fmt.Fprintf(stderr, "scanner: %v\n", runErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintf(stderr, "close resources: %v\n", closeErr)
		return 1
	}
	return 0
}
