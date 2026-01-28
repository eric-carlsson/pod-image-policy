// Package main is the entrypoint for the webhook server.
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

	"github.com/eric-carlsson/pod-image-policy/pkg/admission"
)

type config struct {
	addr       string
	debug      bool
	certFile   string
	keyFile    string
	policyFile string
}

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, logWriter io.Writer) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()
	if len(args) < 1 {
		return errors.New("no arguments provided")
	}

	var cfg config
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	fs.StringVar(&cfg.addr, "addr", ":9443", "Server address")
	fs.BoolVar(&cfg.debug, "debug", false, "Enable debug output")
	fs.StringVar(&cfg.certFile, "certFile", "", "Path to TLS certificate file")
	fs.StringVar(&cfg.keyFile, "keyFile", "", "Path to TLS key file")
	fs.StringVar(&cfg.policyFile, "policyFile", "", "Path to policy config file")
	fs.Parse(args[1:]) //nolint:errcheck,gosec // Potential errors are handled by flag.ExitOnError

	if cfg.certFile == "" || cfg.keyFile == "" {
		return errors.New("certFile and keyFile options must be specified")
	}

	logOpts := slog.HandlerOptions{Level: slog.LevelInfo}
	if cfg.debug {
		logOpts = slog.HandlerOptions{Level: slog.LevelDebug, AddSource: true}
	}
	log := slog.New(slog.NewTextHandler(logWriter, &logOpts))

	runtime, err := admission.NewRuntime(cfg.policyFile)
	if err != nil {
		return fmt.Errorf("new admission runtime: %w", err)
	}

	// Start file watcher in background
	go func() {
		if err := runtime.Watch(ctx, log); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("policy file watcher stopped", "err", err)
		}
	}()

	return startServer(ctx, log, cfg.addr, cfg.certFile, cfg.keyFile, runtime)
}
