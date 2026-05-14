// Package util contains tiny cross-cutting helpers shared by multiple
// packages. Keep this package small; if it grows, split it.
package util

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// EnsureDir creates dir (and parents) if it does not exist.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// downloadHTTPClient has a bounded timeout so a stalled Telegram file
// download cannot hang the bot indefinitely.
var downloadHTTPClient = &http.Client{Timeout: 60 * time.Second}

// DownloadToFile streams an HTTP body into a local file. Bounded by
// downloadHTTPClient.Timeout (60s) and an additional explicit request
// context. The caller owns the resulting path.
func DownloadToFile(url string, destPath string) error {
	if err := EnsureDir(filepath.Dir(destPath)); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
