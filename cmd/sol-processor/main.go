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
	"sync"
	"syscall"

	"github.com/Stone-afk/sol-scanner-service/internal/config"
	"github.com/Stone-afk/sol-scanner-service/internal/ioc"
	iocprocessor "github.com/Stone-afk/sol-scanner-service/internal/ioc/processor"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv))
}

// processorRuntime holds apply/publish starters and a resource closer.
type processorRuntime struct {
	hasUnresolvedHalt func(context.Context) (bool, error)
	startApply        func(context.Context) error
	startPublish      func(context.Context) error
	close             func() error
}

// buildRuntime assembles production dependencies. Tests replace this to inject
// fakes and avoid real Kafka/PostgreSQL.
var buildRuntime = defaultBuildRuntime

func defaultBuildRuntime(ctx context.Context, cfg config.ProcessorConfig, logger *slog.Logger) (processorRuntime, error) {
	db, err := ioc.InitDB(ctx, cfg.Database)
	if err != nil {
		return processorRuntime{}, err
	}
	consumer, err := iocprocessor.InitBlockConsumer(cfg.Kafka, logger)
	if err != nil {
		_ = ioc.CloseDB(db)
		return processorRuntime{}, err
	}
	publisher, err := iocprocessor.InitTransactionProducer(cfg.Kafka, db, cfg.Chain.Name, logger)
	if err != nil {
		_ = consumer.Close()
		_ = ioc.CloseDB(db)
		return processorRuntime{}, err
	}
	proc := iocprocessor.InitProcessor(consumer, db, cfg, logger)
	return processorRuntime{
		hasUnresolvedHalt: func(c context.Context) (bool, error) {
			return iocprocessor.HasUnresolvedHalt(c, db)
		},
		startApply:   proc.Apply,
		startPublish: publisher.Run,
		close: func() error {
			publisher.Close()
			closeErr := consumer.Close()
			dbErr := ioc.CloseDB(db)
			if closeErr != nil {
				return closeErr
			}
			return dbErr
		},
	}, nil
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	flags := flag.NewFlagSet("sol-processor", flag.ContinueOnError)
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

	cfg, err := config.LoadForProcessor(*configPath, lookupEnv)
	if err != nil {
		fmt.Fprintf(stderr, "load configuration: %v\n", err)
		return 1
	}

	logger := slog.New(slog.NewTextHandler(stdout, nil)).With("command", "sol-processor")
	rt, err := buildRuntime(ctx, cfg, logger)
	if err != nil {
		fmt.Fprintf(stderr, "initialize processor: %v\n", err)
		return 1
	}

	halted, err := rt.hasUnresolvedHalt(ctx)
	if err != nil {
		_ = rt.close()
		fmt.Fprintf(stderr, "check halt: %v\n", err)
		return 1
	}

	runErr := func() error {
		runCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		var wg sync.WaitGroup
		errCh := make(chan error, 2)

		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- rt.startPublish(runCtx)
		}()
		alive := 1

		if !halted {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errCh <- rt.startApply(runCtx)
			}()
			alive++
		}

		var first error
		for alive > 0 {
			select {
			case <-ctx.Done():
				cancel()
				wg.Wait()
				return nil
			case e := <-errCh:
				alive--
				if e != nil && !errors.Is(e, context.Canceled) && !errors.Is(e, context.DeadlineExceeded) {
					if first == nil {
						first = e
					}
					cancel()
					wg.Wait()
					return first
				}
				// Apply returned nil after halt (or cancel): publish keeps running until signal.
			}
		}
		return first
	}()
	closeErr := rt.close()

	if runErr != nil {
		fmt.Fprintf(stderr, "processor: %v\n", runErr)
		return 1
	}
	if closeErr != nil {
		fmt.Fprintf(stderr, "close resources: %v\n", closeErr)
		return 1
	}
	return 0
}
