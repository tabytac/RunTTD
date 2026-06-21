package platform

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// ProgressFunc reports download progress; total is -1 when the size is unknown.
type ProgressFunc func(done, total int64)

// progressReader forwards byte counts to a ProgressFunc as the body is read.
type progressReader struct {
	r     io.Reader
	total int64
	done  int64
	cb    ProgressFunc
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.done += int64(n)
		p.cb(p.done, p.total)
	}
	return n, err
}

// downloadAndExtractTo fetches url into archivePath, extracts it into downloadDir,
// and removes the archive. It owns a per-attempt timeout and cleans up the partial
// archive on any failure, so callers can simply loop over candidate URLs and move
// on when it returns a non-nil error. progress may be nil.
func downloadAndExtractTo(ctx context.Context, client *http.Client, url, archivePath, downloadDir string, logger *Logger, progress ProgressFunc) error {
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
	var src io.Reader = resp.Body
	if progress != nil {
		src = &progressReader{r: resp.Body, total: resp.ContentLength, cb: progress}
	}
	if _, err = io.Copy(file, src); err != nil {
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
