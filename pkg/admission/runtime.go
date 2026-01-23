package admission

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eric-carlsson/pod-image-policy/pkg/policy"
	"github.com/fsnotify/fsnotify"
)

// Runtime provides thread-safe access to a validator and mutator loaded from a policy file.
// It supports dynamic reloading via the Reload method or automatic watching via Watch.
type Runtime struct {
	path string
	val  atomic.Pointer[Validator]
	mut  atomic.Pointer[Mutator]
}

// NewRuntime returns a new runtime using the initial policy file
// at path. If the policy can't be loaded, it returns an error.
func NewRuntime(path string) (*Runtime, error) {
	runtime := &Runtime{
		path: path,
	}

	if err := runtime.Reload(); err != nil {
		return nil, err
	}

	return runtime, nil
}

// Validator returns the active validator.
func (r *Runtime) Validator() *Validator {
	return r.val.Load()
}

// Mutator returns the active mutator.
func (r *Runtime) Mutator() *Mutator {
	return r.mut.Load()
}

// Reload the policy file.
func (r *Runtime) Reload() error {
	pol, err := policy.Load(r.path)
	if err != nil {
		return fmt.Errorf("load policy: %w", err)
	}

	r.val.Store(&Validator{Policy: &pol.Validate})
	r.mut.Store(&Mutator{Policy: &pol.Mutate})

	return nil
}

// Watch monitors the policy file for changes and reloads when modified.
// If the new policy file can't be loaded, it logs the error but
// does not return. The old policy continues to be active.
func (r *Runtime) Watch(ctx context.Context, log *slog.Logger) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	dir := filepath.Dir(r.path)
	// Watch ..data to catch changes in volumes mounted from ConfigMap, etc.
	// See https://github.com/kubernetes/kubernetes/blob/c2618d48c0623bec76c0709bd01ea87eb6e0cde3/pkg/volume/util/atomic_writer.go#L39-L57
	watchTargets := []string{dir, filepath.Join(dir, "..data")}

	for _, target := range watchTargets {
		if err := watcher.Add(target); err != nil {
			log.Warn("failed to add watch target", "path", target, "err", err)
		}
	}

	log.Info("watching policy file for changes", "path", r.path, "watchDir", dir)

	// Debounce rapid file changes
	var reloadTimer *time.Timer
	const debounceDelay = 100 * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			log.Info("stopping policy file watcher")
			return ctx.Err()

		case event, ok := <-watcher.Events:
			if !ok {
				return fmt.Errorf("watcher events channel closed")
			}

			if !shouldReloadFile(event, r.path) {
				continue
			}

			// Debounce: reset timer on each event
			if reloadTimer != nil {
				reloadTimer.Stop()
			}
			reloadTimer = time.AfterFunc(debounceDelay, func() {
				log.Info("config file changed, reloading", "path", r.path, "event", event.Op)
				if err := r.Reload(); err != nil {
					log.Error("failed to reload config", "err", err)
				} else {
					log.Info("config reloaded successfully")
				}
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				return fmt.Errorf("watcher errors channel closed")
			}
			log.Error("watcher error", "err", err)
		}
	}
}

// shouldReloadFile determines if an fsnotify event should trigger a config reload.
func shouldReloadFile(event fsnotify.Event, configPath string) bool {
	// Only care about write/create/remove/rename/chmod events
	if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename|fsnotify.Chmod) == 0 {
		return false
	}

	cleanConfig := filepath.Clean(configPath)
	cleanEvent := filepath.Clean(event.Name)

	// Direct match on config file
	if cleanEvent == cleanConfig {
		return true
	}

	// ConfigMap volumes update ..data symlink and its contents
	if strings.Contains(cleanEvent, string(filepath.Separator)+"..data") {
		return true
	}

	return false
}
