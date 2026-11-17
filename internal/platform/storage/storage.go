// Package storage stores uploaded files such as medical reports.
//
// Local keeps files on disk for development. A production deployment should
// use an S3-compatible bucket with server-side encryption behind the same interface.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

type Storage interface {
	// Put writes r under key and returns the number of bytes written.
	Put(ctx context.Context, key string, r io.Reader) (int64, error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

var validKey = regexp.MustCompile(`^[a-z0-9-]{8,64}$`)

var ErrInvalidKey = errors.New("storage: invalid key")

type Local struct {
	dir string
}

func NewLocal(dir string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("create upload dir: %w", err)
	}
	return &Local{dir: dir}, nil
}

func (l *Local) path(key string) (string, error) {
	if !validKey.MatchString(key) {
		return "", ErrInvalidKey
	}
	return filepath.Join(l.dir, key), nil
}

func (l *Local) Put(_ context.Context, key string, r io.Reader) (int64, error) {
	p, err := l.path(key)
	if err != nil {
		return 0, err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(p)
		return 0, err
	}
	return n, nil
}

func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (l *Local) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
