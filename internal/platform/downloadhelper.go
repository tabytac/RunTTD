package platform

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// downloadAndExtractTo fetches url into archivePath, extracts it into downloadDir,
// and removes the archive. It owns a per-attempt timeout and cleans up the partial
// archive on any failure, so callers can simply loop over candidate URLs and move
// on when it returns a non-nil error.
func downloadAndExtractTo(ctx context.Context, client *http.Client, url, archivePath, downloadDir string, logger *Logger) error {
	dlCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	resp, err := doGETWithRetry(dlCtx, client, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	file, err := os.Create(archivePath)
	if err != nil {
		return err
	}
	if _, err = io.Copy(file, resp.Body); err != nil {
		file.Close()
		os.Remove(archivePath)
		return err
	}
	file.Close()

	if err := ExtractArchive(archivePath, downloadDir, logger); err != nil {
		os.Remove(archivePath)
		return err
	}
	os.Remove(archivePath)
	return nil
}
