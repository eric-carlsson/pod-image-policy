package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"
)

type Config struct {
	Addr     string
	certFile string
	keyFile  string
}

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Args, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, logWriter io.Writer) error {
	if len(args) < 1 {
		return errors.New("no arguments provided")
	}

	var config Config
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	fs.StringVar(&config.Addr, "addr", ":9443", "Server address")
	fs.StringVar(&config.certFile, "certFile", "", "Path to TLS certificate file")
	fs.StringVar(&config.keyFile, "keyFile", "", "Path to TLS key file")
	fs.Parse(args[1:])

	if config.certFile == "" || config.keyFile == "" {
		return errors.New("certFile and keyFile options must be specified")
	}

	log := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug}))

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/mutate", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world!\n"))
		w.WriteHeader(200)
	})

	srv := http.Server{
		Addr:     config.Addr,
		Handler:  mux,
		ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	shutdownErr := make(chan error)

	go func() {
		<-ctx.Done()
		log.Info("shutting down server", "err", ctx.Err())

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		shutdownErr <- srv.Shutdown(ctx)
	}()

	log.Info("starting server", "addr", config.Addr)

	if err := srv.ListenAndServeTLS(config.certFile, config.keyFile); err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve: %w", err)
	}

	if err := <-shutdownErr; err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	return nil
}
