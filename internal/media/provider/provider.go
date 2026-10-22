// Package provider is the upload plugin's storage interface and the local
// and S3 providers.
package provider

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// File is what a provider stores: Hash and Ext make the name, Path is the
// optional folder prefix. Upload sets URL, and Metadata when the provider
// keeps some (provider_metadata, given back on delete).
type File struct {
	Hash, Ext, Mime, Path string
	Size                  int64
	URL                   string
	Metadata              map[string]any
}

// Provider stores and removes files.
type Provider interface {
	Upload(ctx context.Context, f *File, r io.Reader) error
	Delete(ctx context.Context, f *File) error
	// IsPrivate reports whether URLs must be signed to be read.
	IsPrivate() bool
	SignedURL(ctx context.Context, f *File) (string, error)
}

// SizeChecker is the optional checkFileSize of a provider: it replaces
// the sizeLimit check of the upload plugin.
type SizeChecker interface {
	CheckFileSize(f *File, sizeLimit int64) error
}

// Local writes to <public>/uploads, served at /uploads.
type Local struct {
	PublicDir string
}

func (l *Local) dir() string { return filepath.Join(l.PublicDir, "uploads") }

// Upload writes atomically: a temporary file in the same folder, renamed.
func (l *Local) Upload(_ context.Context, f *File, r io.Reader) error {
	if err := os.MkdirAll(l.dir(), 0o750); err != nil {
		return err
	}
	name := filepath.Base(f.Hash + f.Ext)
	tmp, err := os.CreateTemp(l.dir(), ".upload-")
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(l.dir(), name)); err != nil {
		return err
	}
	f.URL = "/uploads/" + name
	return nil
}

// Delete removes the file; a missing file is not an error.
func (l *Local) Delete(_ context.Context, f *File) error {
	err := os.Remove(filepath.Join(l.dir(), filepath.Base(f.Hash+f.Ext)))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// IsPrivate is false: /uploads is public.
func (l *Local) IsPrivate() bool { return false }

// SignedURL returns the plain URL.
func (l *Local) SignedURL(_ context.Context, f *File) (string, error) { return f.URL, nil }
