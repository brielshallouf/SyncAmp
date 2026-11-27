// Package notify posts JSON events to a webhook (Slack/Teams/ntfy-compatible
// receivers can consume the "text" field).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Send POSTs {"text": msg} to url. Empty url is a no-op.
func Send(ctx context.Context, url, msg string) error {
	if url == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	b, _ := json.Marshal(map[string]string{"text": "[SyncAmp] " + msg})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}
