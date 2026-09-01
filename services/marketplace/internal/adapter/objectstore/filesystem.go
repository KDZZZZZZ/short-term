// Package objectstore implements the Marketplace ObjectStore port.
//
// docs/software-design.md section 9.2 puts product images in object storage
// reached only by this adapter. The filesystem implementation here is what
// local development and integration tests run against; an Alibaba Cloud OSS
// implementation satisfies the same interface and changes no business code.
package objectstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Filesystem stores objects under a base directory and serves them from a
// public base URL.
type Filesystem struct {
	root      string
	publicURL string
}

// ErrUnsafeKey reports a key that would escape the storage root.
var ErrUnsafeKey = errors.New("objectstore: unsafe object key")

// NewFilesystem creates the storage root if needed and returns a store.
// publicURL is the prefix the outside world uses to fetch objects.
func NewFilesystem(root, publicURL string) (*Filesystem, error) {
	if root == "" {
		return nil, errors.New("objectstore: storage root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("objectstore: resolve root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o750); err != nil {
		return nil, fmt.Errorf("objectstore: create root: %w", err)
	}
	return &Filesystem{root: absolute, publicURL: strings.TrimSuffix(publicURL, "/")}, nil
}

// Put writes an object, replacing any existing one with the same key.
//
// The write goes to a temporary file that is then renamed, so a reader never
// observes a half-written image and a failed write leaves the previous object
// intact.
func (f *Filesystem) Put(_ context.Context, key, _ string, data []byte) error {
	target, err := f.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fmt.Errorf("objectstore: create directory: %w", err)
	}

	temp, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return fmt.Errorf("objectstore: create temporary file: %w", err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("objectstore: write object: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("objectstore: close object: %w", err)
	}
	if err := os.Chmod(tempName, 0o640); err != nil {
		return fmt.Errorf("objectstore: set object permissions: %w", err)
	}
	if err := os.Rename(tempName, target); err != nil {
		return fmt.Errorf("objectstore: publish object: %w", err)
	}
	return nil
}

// Delete removes an object. A missing object is not an error, so cleanup and
// retries are idempotent.
func (f *Filesystem) Delete(_ context.Context, key string) error {
	target, err := f.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("objectstore: delete object: %w", err)
	}
	return nil
}

// URL returns the public location of an object.
func (f *Filesystem) URL(key string) string {
	if key == "" {
		return ""
	}
	return f.publicURL + "/" + key
}

// Root reports the storage directory, so a deployment can mount and serve it.
func (f *Filesystem) Root() string { return f.root }

// resolve turns an object key into a path inside the root, rejecting anything
// that would escape it. Keys are server-generated today; this check is what
// keeps that from silently becoming a path traversal if that ever changes.
func (f *Filesystem) resolve(key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return "", ErrUnsafeKey
	}
	target := filepath.Join(f.root, filepath.FromSlash(key))
	if target != f.root && !strings.HasPrefix(target, f.root+string(filepath.Separator)) {
		return "", ErrUnsafeKey
	}
	return target, nil
}
