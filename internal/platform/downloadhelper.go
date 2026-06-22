package platform

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
func downloadAndExtractTo(ctx context.Context, client *http.Client, url, archivePath, downloadDir, osTag string, logger *Logger, progress ProgressFunc) error {
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

	if err := extractIntoVersionFolder(archivePath, downloadDir, osTag, logger); err != nil {
		os.Remove(archivePath)
		return err
	}
	os.Remove(archivePath)
	return nil
}

// extractIntoVersionFolder extracts archivePath into downloadDir as a single
// versioned subfolder. Modern archives already wrap their files in one top-level
// folder, which is moved up as-is. Older flat archives (some old JGRPP MINGW
// builds) extract loose files with no enclosing folder, so they are wrapped in a
// folder named after the archive, keeping the download dir tidy and findable.
func extractIntoVersionFolder(archivePath, downloadDir, osTag string, logger *Logger) error {
	tmp, err := os.MkdirTemp(downloadDir, ".extract-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	if err := ExtractArchive(archivePath, tmp, logger); err != nil {
		return err
	}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("archive %s extracted no files", filepath.Base(archivePath))
	}

	src := tmp
	name := archiveBaseName(archivePath)
	if len(entries) == 1 && entries[0].IsDir() {
		src = filepath.Join(tmp, entries[0].Name())
		name = entries[0].Name()
	}

	name = ensureOSTag(name, osTag)
	dst := filepath.Join(downloadDir, name)
	os.RemoveAll(dst) // replace any earlier copy
	return os.Rename(src, dst)
}

// archiveBaseName is the archive's file name without its extension.
func archiveBaseName(archivePath string) string {
	n := filepath.Base(archivePath)
	for _, ext := range []string{".tar.xz", ".tar.bz2", ".tar.gz", ".zip", ".dmg"} {
		if strings.HasSuffix(n, ext) {
			return strings.TrimSuffix(n, ext)
		}
	}
	return n
}

// ensureOSTag appends the matched OS tag when the extracted folder name lacks one,
// so tagless old archives (e.g. 0.1.0) are re-discoverable after installation.
func ensureOSTag(name, osTag string) string {
	if osTag == "" {
		return name
	}
	lower := strings.ToLower(name)
	for _, tag := range []string{"windows-", "mingw-", "macos-universal", "macosx-universal", "linux-"} {
		if strings.Contains(lower, tag) {
			return name
		}
	}
	return name + "-" + osTag
}
