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
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eric-carlsson/pod-image-policy/pkg/config"
	"github.com/eric-carlsson/pod-image-policy/pkg/mutate"
	"github.com/fsnotify/fsnotify"

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

	mutator, err := loadMutator(log, srvCfg.configFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	var mutatorPtr atomic.Pointer[mutate.Mutator]
	mutatorPtr.Store(mutator)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/mutate", mutateHandler(log, &mutatorPtr))

	srv := http.Server{
		Addr:     srvCfg.Addr,
		Handler:  mux,
		ErrorLog: slog.NewLogLogger(log.Handler(), slog.LevelError),
	}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	if srvCfg.configFile != "" {
		go watchConfig(ctx, log, srvCfg.configFile, func(newMutator *mutate.Mutator) {
			mutatorPtr.Store(newMutator)
		})
	}

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

func loadMutator(log *slog.Logger, path string) (*mutate.Mutator, error) {
	admissionConfig, err := config.Load(path)
	if err != nil {
		return nil, err
	}

	mutator, err := mutate.NewMutator(admissionConfig.Mutate)
	if err != nil {
		return nil, fmt.Errorf("compile mutate rules: %w", err)
	}

	log.Info("admission config (re)loaded",
		"path", path,
		"mutateRules", len(admissionConfig.Mutate.Rules),
		"validateRules", len(admissionConfig.Validate.Rules),
	)

	return mutator, nil
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

func mutateHandler(log *slog.Logger, mutatorPtr *atomic.Pointer[mutate.Mutator]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		review, err := decode[admissionv1.AdmissionReview](r)
		if err != nil {
			log.Error("failed to decode admission review", "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		mutator := mutatorPtr.Load()
		if mutator == nil {
			mutator = &mutate.Mutator{}
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

func watchConfig(ctx context.Context, log *slog.Logger, path string, onReload func(*mutate.Mutator)) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Error("config watcher start failed", "err", err)
		return
	}
	defer watcher.Close()

	var lastReload atomic.Int64

	dir := filepath.Dir(path)
	watchTargets := []string{dir, filepath.Join(dir, "..data")}
	for _, target := range watchTargets {
		if err := watcher.Add(target); err != nil {
			log.Error("config watcher add failed", "path", target, "err", err)
		}
	}

	log.Info("config reload watcher enabled", "file", path)

	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-watcher.Events:
			if !shouldReload(evt, path) {
				continue
			}

			now := time.Now().UnixNano()
			prev := lastReload.Load()
			// Add debounce to prevent repeated reloads during single update
			if prev != 0 && time.Duration(now-prev) < 200*time.Millisecond {
				continue
			}
			lastReload.Store(now)

			newMutator, err := loadMutator(log, path)
			if err != nil {
				log.Error("config reload failed", "err", err)
				continue
			}

			onReload(newMutator)
		case err := <-watcher.Errors:
			log.Error("config watcher error", "err", err)
		}
	}
}

func shouldReload(evt fsnotify.Event, cfgPath string) bool {
	if evt.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove|fsnotify.Chmod) == 0 {
		return false
	}

	cleanCfg := filepath.Clean(cfgPath)
	cleanEvt := filepath.Clean(evt.Name)

	if cleanEvt == cleanCfg {
		return true
	}

	// ConfigMap volumes update ..data symlink and its contents
	if strings.Contains(cleanEvt, string(filepath.Separator)+"..data") {
		return true
	}

	return false
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
