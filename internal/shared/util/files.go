// Package util contains tiny cross-cutting helpers shared by multiple
// packages. Keep this package small; if it grows, split it.
package util

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// EnsureDir creates dir (and parents) if it does not exist.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// DownloadToFile streams an HTTP body into a local file. Used by the
// Telegram bot to fetch voice files from Telegram's file API. The
// caller owns the resulting path.
func DownloadToFile(url string, destPath string) error {
	if err := EnsureDir(filepath.Dir(destPath)); err != nil {
		return err
	}
	resp, err := http.Get(url) //nolint:gosec // url comes from Telegram API
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
