package ip

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Fetcher struct {
	client *http.Client
	ua     string
}

func NewFetcher(timeout time.Duration, userAgent string) *Fetcher {
	return &Fetcher{
		client: &http.Client{
			Timeout: timeout,
		},
		ua: userAgent,
	}
}

func (f *Fetcher) Fetch(ctx context.Context, url string) (net.IP, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if strings.TrimSpace(f.ua) != "" {
		req.Header.Set("User-Agent", f.ua)
	}
	req.Header.Set("Accept", "text/plain")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	ipStr := strings.TrimSpace(string(body))
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP response: %q", ipStr)
	}
	return ip, nil
}
