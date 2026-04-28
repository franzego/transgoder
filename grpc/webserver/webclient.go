package webserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/franzego/transcoder/grpc/config"
	"github.com/franzego/transcoder/grpc/retry"
	"github.com/franzego/transcoder/grpc/weberror"
)

type WebserverClient struct {
	client  *http.Client
	cfg     *config.Config
	retryer *retry.Retry
}

func NewWebserverClient(cfg *config.Config) *WebserverClient {
	return &WebserverClient{
		client:  &http.Client{Timeout: 10 * time.Second},
		cfg:     cfg,
		retryer: retry.NewRetry(),
	}
}

// UpdateJobStatus sends a request to the web server to update the status of a job.
func (wc *WebserverClient) UpdateJobStatus(ctx context.Context, req JobStatusReq) (JobStatusResponse, error) {
	var out JobStatusResponse
	err := wc.retryer.Do(ctx, "update_job_status", func() error {
		url := wc.cfg.WebServer.ServerUrl
		path := fmt.Sprintf("%s/status/%s/update", url, req.JobID)
		buf, err := json.Marshal(&req)
		if err != nil {
			return fmt.Errorf("failed to marshal body: %w", err)
		}
		body := bytes.NewBuffer(buf)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, path, body)
		if err != nil {
			return fmt.Errorf("failed to create http request: %w", err)
		}
		httpReq.Header.Set("Content-type", "application/json")

		resp, err := wc.client.Do(httpReq)
		if err != nil {
			return &weberror.RequestError{Message: "could not reach web server", Err: err}
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return &weberror.RequestError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("webserver returned status code %d", resp.StatusCode)}
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return fmt.Errorf("error getting response from webserver: %w", err)
		}
		return nil
	})
	if err != nil {
		return JobStatusResponse{}, err
	}
	return out, nil
}

// GetSourceURL retrieves the source URL from web server.
func (wc *WebserverClient) GetSourceURL(ctx context.Context, jobID string) (string, error) {
	var sourceURL string
	err := wc.retryer.Do(ctx, "get_source_url", func() error {
		path := fmt.Sprintf("%s/jobs/%s/source-url", wc.cfg.WebServer.ServerUrl, jobID)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
		if err != nil {
			return fmt.Errorf("failed to create http request: %w", err)
		}

		resp, err := wc.client.Do(httpReq)
		if err != nil {
			return &weberror.RequestError{Message: "could not reach web server", Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return &weberror.RequestError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("webserver returned status code %d", resp.StatusCode)}
		}

		var out SourceURLResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return fmt.Errorf("error decoding source-url response: %w", err)
		}
		if out.Metadata.SourceURL == "" {
			return fmt.Errorf("webserver source_url is empty")
		}
		sourceURL = out.Metadata.SourceURL
		return nil
	})
	if err != nil {
		return "", err
	}
	return sourceURL, nil
}

// GetTranscodeProfile retrieves the transcode profile from web server.
func (wc *WebserverClient) GetTranscodeProfile(ctx context.Context, jobID string) (map[string]any, error) {
	var metadata map[string]any
	err := wc.retryer.Do(ctx, "get_transcode_profile", func() error {
		// /jobs/{id}/transcode-profile
		path := fmt.Sprintf("%s/jobs/%s/transcode-profile", wc.cfg.WebServer.ServerUrl, jobID)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
		if err != nil {
			return fmt.Errorf("failed to create http request: %w", err)
		}

		resp, err := wc.client.Do(httpReq)
		if err != nil {
			return &weberror.RequestError{Message: "could not reach web server", Err: err}
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return &weberror.RequestError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("webserver returned status code %d", resp.StatusCode)}
		}
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return fmt.Errorf("error decoding transcode profile response: %w", err)
		}
		if m, ok := out["metadata"].(map[string]any); ok {
			metadata = m
			return nil
		}
		return fmt.Errorf("transcode profile response missing metadata field")
	})
	if err != nil {
		return nil, err
	}
	return metadata, nil
}
