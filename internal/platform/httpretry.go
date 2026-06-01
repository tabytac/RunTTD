package platform

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

// downloadClient is used for large archive transfers; it has no total timeout so a slow body transfer is never capped, relying on transport-level timeouts and the caller's context deadline instead
var downloadClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 15 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

var retryBackoffs = []time.Duration{1 * time.Second, 2 * time.Second}

func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

func retryAfterDelay(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return 0
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// doGETWithRetry performs a GET on url, retrying only transient failures (transport errors, 429, 5xx) with context-aware backoff; non-retryable responses such as 404 are returned unchanged, and the caller must close the returned Body
func doGETWithRetry(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	var lastErr error
	totalAttempts := len(retryBackoffs) + 1

	// wait before the next attempt
	var nextDelay time.Duration

	for attempt := 0; attempt < totalAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, nextDelay); err != nil {
				if lastErr != nil {
					return nil, lastErr
				}
				return nil, err
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}

		if attempt < len(retryBackoffs) {
			nextDelay = retryBackoffs[attempt]
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if isRetryableStatus(resp.StatusCode) {
			lastErr = fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
			if ra := retryAfterDelay(resp); ra > nextDelay {
				nextDelay = ra
			}
			resp.Body.Close()
			continue
		}

		return resp, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("GET %s: exhausted retries", url)
	}
	return nil, lastErr
}
