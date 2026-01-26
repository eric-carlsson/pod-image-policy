package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/eric-carlsson/pod-image-policy/pkg/admission"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func startServer(ctx context.Context, log *slog.Logger, addr, certFile, keyFile string, runtime *admission.Runtime) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", okHandler)
	mux.HandleFunc("/ready", okHandler)
	mux.Handle("/mutate", newAdmissionHandler(log, runtime, handleMutation))
	mux.Handle("/validate", newAdmissionHandler(log, runtime, handleValidation))

	srv := http.Server{
		Addr:         addr,
		Handler:      mux,
		ErrorLog:     slog.NewLogLogger(log.Handler(), slog.LevelError),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	shutdownErr := make(chan error)

	go func() {
		<-ctx.Done()
		log.Info("shutting down server", "err", ctx.Err())

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		shutdownErr <- srv.Shutdown(ctx)
	}()

	log.Info("starting server", "addr", addr)

	if err := srv.ListenAndServeTLS(certFile, keyFile); err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve: %w", err)
	}

	if err := <-shutdownErr; err != nil {
		return fmt.Errorf("shutdown server: %w", err)
	}

	return nil
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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

type admissionInner func(uid types.UID, pod *corev1.Pod, runtime *admission.Runtime) (*admissionv1.AdmissionResponse, error)

type admissionHandler struct {
	log     *slog.Logger
	runtime *admission.Runtime
	inner   admissionInner
}

func newAdmissionHandler(log *slog.Logger, runtime *admission.Runtime, inner admissionInner) http.Handler {
	return admissionHandler{log: log, runtime: runtime, inner: inner}
}

func (h admissionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	review, err := decode[admissionv1.AdmissionReview](r)
	if err != nil {
		h.log.Error("failed to decode admission review", "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.log.Info("admission request received",
		"uid", review.Request.UID,
		"kind", review.Request.Kind,
		"resource", review.Request.Resource,
		"namespace", review.Request.Namespace,
		"name", review.Request.Name,
	)

	var pod corev1.Pod
	if err := json.Unmarshal(review.Request.Object.Raw, &pod); err != nil {
		h.log.Error("failed to decode pod", "err", err)
		http.Error(w, fmt.Sprintf("decode pod: %s", err.Error()), http.StatusBadRequest)
		return
	}

	resp, err := h.inner(review.Request.UID, &pod, h.runtime)
	if err != nil {
		h.log.Error("failed to handle admission", "err", err)
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
		h.log.Error("failed to encode admission response", "err", err)
	}
}

func handleMutation(uid types.UID, pod *corev1.Pod, runtime *admission.Runtime) (*admissionv1.AdmissionResponse, error) {
	res, err := runtime.Mutator().MutatePodImages(pod)
	if err != nil {
		return nil, err
	}
	response := &admissionv1.AdmissionResponse{
		UID:     uid,
		Allowed: true,
	}

	if len(res.Patches) > 0 {
		patchBytes, err := json.Marshal(res.Patches)
		if err != nil {
			return nil, fmt.Errorf("marshal patch: %w", err)
		}
		pt := admissionv1.PatchTypeJSONPatch
		response.PatchType = &pt
		response.Patch = patchBytes
	}

	if len(res.Warnings) > 0 {
		response.Warnings = res.Warnings
	}

	return response, nil
}

func handleValidation(uid types.UID, pod *corev1.Pod, runtime *admission.Runtime) (*admissionv1.AdmissionResponse, error) {
	result, err := runtime.Validator().ValidatePodImages(pod)
	if err != nil {
		return nil, err
	}

	response := &admissionv1.AdmissionResponse{
		UID:     uid,
		Allowed: result.Allowed,
	}

	if !result.Allowed {
		response.Result = &metav1.Status{Message: result.Message}
	}

	if len(result.Warnings) > 0 {
		response.Warnings = result.Warnings
	}

	return response, nil
}
