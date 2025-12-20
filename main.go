package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	mux.HandleFunc("/mutate", mutateHandler(log))

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

func encode[T any](w http.ResponseWriter, status int, v T) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func decode[T any](r *http.Request) (T, error) {
	var v T
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		return v, fmt.Errorf("unexpected content-type: %s", ct)
	}
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		return v, fmt.Errorf("decode json: %w", err)
	}
	return v, nil
}

func mutateHandler(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		review, err := decode[admissionv1.AdmissionReview](r)
		if err != nil {
			log.Error("failed to decode admission review", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := handleMutation(&review, log)
		if err != nil {
			log.Error("failed to handle mutation", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		response := admissionv1.AdmissionReview{
			TypeMeta: metav1.TypeMeta{
				APIVersion: admissionv1.SchemeGroupVersion.String(),
				Kind:       "AdmissionReview",
			},
			Response: resp,
		}

		if err := encode(w, http.StatusOK, response); err != nil {
			log.Error("failed to encode admission response", "err", err)
		}
	}
}

// handleMutation performs the admission logic and returns an AdmissionResponse.
func handleMutation(review *admissionv1.AdmissionReview, log *slog.Logger) (*admissionv1.AdmissionResponse, error) {
	if review.Request == nil {
		return nil, errors.New("admission review request is nil")
	}

	log.Info("admission request received",
		"uid", review.Request.UID,
		"kind", review.Request.Kind,
		"resource", review.Request.Resource,
		"namespace", review.Request.Namespace,
		"name", review.Request.Name,
	)

	// TODO: add mutation logic and JSON patch construction here
	return &admissionv1.AdmissionResponse{
		UID:     review.Request.UID,
		Allowed: true,
	}, nil
}
