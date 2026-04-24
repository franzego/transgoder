package webserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/franzego/transcoder/grpc/config"
)

type WebserverClient struct {
	client *http.Client
	cfg    *config.Config
}

func NewWebserverClient(cfg *config.Config) *WebserverClient {
	return &WebserverClient{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		cfg: cfg,
	}
}

// UpdateJobStatus sends a request to the web server to update the status of a job.
func (wc *WebserverClient) UpdateJobStatus(ctx context.Context, req JobStatusReq) (JobStatusResponse, error) {
	url := wc.cfg.WebServer.ServerUrl
	path := fmt.Sprintf("%s/status/%s/update", url, req.JobID)
	buf, err := json.Marshal(&req)
	if err != nil {
		return JobStatusResponse{}, fmt.Errorf("failed to marshal body: %w", err)
	}
	body := bytes.NewBuffer(buf)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", path, body)
	if err != nil {
		return JobStatusResponse{}, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-type", "application/json")

	resp, err := wc.client.Do(httpReq)
	if err != nil {
		return JobStatusResponse{}, &RequestError{
			Message: "could not reach web server",
			Err:     err,
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return JobStatusResponse{}, &RequestError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("webserver returned status code %d", resp.StatusCode),
		}
	}

	var res JobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return JobStatusResponse{}, fmt.Errorf("error getting response from webserver: %w", err)
	}
	return res, nil
}

// GetSourceURL retrieves the source URL which is the presigned url from minio for a given job ID from the web server.
// It constructs the appropriate HTTP request, sends it, and processes the response to extract the source URL.
// If any step fails, it returns an error with details about the failure.
func (wc *WebserverClient) GetSourceURL(ctx context.Context, jobID string) (string, error) {
	path := fmt.Sprintf("%s/jobs/%s/source-url", wc.cfg.WebServer.ServerUrl, jobID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create http request: %w", err)
	}

	resp, err := wc.client.Do(httpReq)
	if err != nil {
		return "", &RequestError{
			Message: "could not reach web server",
			Err:     err,
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", &RequestError{
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("webserver returned status code %d", resp.StatusCode),
		}
	}

	var out SourceURLResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("error decoding source-url response: %w", err)
	}
	if out.Metadata.SourceURL == "" {
		return "", fmt.Errorf("webserver source_url is empty")
	}
	return out.Metadata.SourceURL, nil
}
