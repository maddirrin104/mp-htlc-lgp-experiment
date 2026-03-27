package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPPeerClient struct{ client *http.Client }

func NewHTTPPeerClient(timeout time.Duration) *HTTPPeerClient {
	return &HTTPPeerClient{client: &http.Client{Timeout: timeout}}
}

func (c *HTTPPeerClient) PostJSON(ctx context.Context, baseURL, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("peer %s %s -> status %d: %s", baseURL, path, resp.StatusCode, string(b))
	}
	return nil
}
