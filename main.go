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

	"github.com/eric-carlsson/pod-image-admissiob/internal/config"
	"github.com/eric-carlsson/pod-image-admissiob/internal/mutate"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ServerConfig struct {
	Addr       string
	certFile   string
	keyFile    string
	configFile string
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

	var srvCfg ServerConfig
	fs := flag.NewFlagSet(args[0], flag.ExitOnError)
	fs.StringVar(&srvCfg.Addr, "addr", ":9443", "Server address")
	fs.StringVar(&srvCfg.certFile, "certFile", "", "Path to TLS certificate file")
	fs.StringVar(&srvCfg.keyFile, "keyFile", "", "Path to TLS key file")
	fs.StringVar(&srvCfg.configFile, "configFile", "", "Path to admission config file")
	fs.Parse(args[1:])

	if srvCfg.certFile == "" || srvCfg.keyFile == "" {
		return errors.New("certFile and keyFile options must be specified")
	}

	log := slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug}))

	admissionConfig, err := config.Load(srvCfg.configFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log.Info("admission config loaded",
		"path", srvCfg.configFile,
		"mutateRules", len(admissionConfig.Mutate.Rules),
		"validateRules", len(admissionConfig.Validate.Rules),
	)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/mutate", mutateHandler(log, admissionConfig))

	srv := http.Server{
		Addr:     srvCfg.Addr,
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

	log.Info("starting server", "addr", srvCfg.Addr)

	if err := srv.ListenAndServeTLS(srvCfg.certFile, srvCfg.keyFile); err != http.ErrServerClosed {
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

func mutateHandler(log *slog.Logger, cfg *config.AdmissionConfig) http.HandlerFunc {
	if cfg == nil {
		cfg = &config.AdmissionConfig{}
	}
	config.Default(cfg)

	mutator, err := mutate.NewMutator(cfg.Mutate)
	if err != nil {
		// Fail fast on startup misconfiguration
		panic(fmt.Sprintf("compile mutate rules: %v", err))
	}

	return func(w http.ResponseWriter, r *http.Request) {
		review, err := decode[admissionv1.AdmissionReview](r)
		if err != nil {
			log.Error("failed to decode admission review", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp, err := handleMutation(&review, log, mutator)
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
func handleMutation(review *admissionv1.AdmissionReview, log *slog.Logger, mutator *mutate.Mutator) (*admissionv1.AdmissionResponse, error) {
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

	var pod corev1.Pod
	if err := json.Unmarshal(review.Request.Object.Raw, &pod); err != nil {
		return nil, fmt.Errorf("decode pod: %w", err)
	}

	patches, err := mutator.RewritePodImages(&pod)
	if err != nil {
		return nil, fmt.Errorf("rewrite images: %w", err)
	}

	response := &admissionv1.AdmissionResponse{
		UID:     review.Request.UID,
		Allowed: true,
	}

	if len(patches) > 0 {
		patchBytes, err := json.Marshal(patches)
		if err != nil {
			return nil, fmt.Errorf("marshal patch: %w", err)
		}
		pt := admissionv1.PatchTypeJSONPatch
		response.PatchType = &pt
		response.Patch = patchBytes
	}

	return response, nil
}
