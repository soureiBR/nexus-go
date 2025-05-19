package utils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DownloadMediaFromURL downloads media from a URL and returns it as a byte slice
func DownloadMediaFromURL(url string) ([]byte, error) {
	// Create context with timeout to avoid hanging on slow downloads
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %s", resp.Status)
	}

	// Read the response body and return as a byte slice
	return io.ReadAll(resp.Body)
}
