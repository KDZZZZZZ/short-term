// Package config loads service configuration from the process environment.
//
// Secrets are injected by the runtime (GitHub Actions secrets or the server
// environment) and never committed, so the environment is the only supported
// source. A Loader accumulates problems and reports them together, which makes
// a misconfigured deployment fail fast with one complete message instead of
// one variable at a time.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Loader reads typed values from the environment and collects every problem it
// finds.
type Loader struct {
	lookup func(string) (string, bool)
	errs   []error
}

// NewLoader builds a Loader over os.LookupEnv.
func NewLoader() *Loader { return &Loader{lookup: os.LookupEnv} }

// NewLoaderFrom builds a Loader over an explicit environment, for tests.
func NewLoaderFrom(env map[string]string) *Loader {
	return &Loader{lookup: func(key string) (string, bool) {
		value, ok := env[key]
		return value, ok
	}}
}

// Err returns the joined configuration errors, or nil when everything loaded.
func (l *Loader) Err() error { return errors.Join(l.errs...) }

// String returns the variable or fallback when it is unset or empty.
func (l *Loader) String(key, fallback string) string {
	value, ok := l.lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// Required returns the variable and records an error when it is missing.
func (l *Loader) Required(key string) string {
	value, ok := l.lookup(key)
	if !ok || strings.TrimSpace(value) == "" {
		l.errs = append(l.errs, fmt.Errorf("config: %s is required", key))
		return ""
	}
	return value
}

// Int returns an integer variable or fallback.
func (l *Loader) Int(key string, fallback int) int {
	raw, ok := l.lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("config: %s must be an integer: %w", key, err))
		return fallback
	}
	return value
}

// Duration returns a duration variable or fallback, accepting Go syntax such
// as "250ms" or "5s".
func (l *Loader) Duration(key string, fallback time.Duration) time.Duration {
	raw, ok := l.lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("config: %s must be a duration: %w", key, err))
		return fallback
	}
	return value
}

// Bool returns a boolean variable or fallback.
func (l *Loader) Bool(key string, fallback bool) bool {
	raw, ok := l.lookup(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("config: %s must be a boolean: %w", key, err))
		return fallback
	}
	return value
}

// Fail records a validation problem discovered after loading, so callers can
// report configuration and cross-field errors through one channel.
func (l *Loader) Fail(format string, args ...any) {
	l.errs = append(l.errs, fmt.Errorf(format, args...))
}
