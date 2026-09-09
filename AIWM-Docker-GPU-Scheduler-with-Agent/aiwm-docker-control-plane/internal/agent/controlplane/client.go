package controlplane

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent"
	agentv1 "github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agentprotocol/v1"
)

type TLSConfig struct {
	CAFile   string
	CertFile string
	KeyFile  string
}

type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func New(rawURL string, timeout time.Duration, tlsFiles TLSConfig) (*Client, error) {
	baseURL, err := url.Parse(strings.TrimRight(rawURL, "/"))
	if err != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid Control Plane URL %q", rawURL)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	tlsConfig, err := loadTLSConfig(tlsFiles)
	if err != nil {
		return nil, err
	}
	transport.TLSClientConfig = tlsConfig
	return &Client{baseURL: baseURL, httpClient: &http.Client{Transport: transport, Timeout: timeout}}, nil
}

func (c *Client) Register(ctx context.Context, request agentv1.RegisterRequest, enrollmentToken string) (agentv1.RegisterResponse, error) {
	return call[agentv1.RegisterResponse](ctx, c, http.MethodPost, "/api/v1/agents/register", request, map[string]string{
		agentv1.EnrollmentTokenHeader: enrollmentToken,
	})
}

func (c *Client) Heartbeat(ctx context.Context, agentID, token string, request agentv1.HeartbeatRequest) error {
	_, err := call[json.RawMessage](ctx, c, http.MethodPost, "/api/v1/agents/"+url.PathEscape(agentID)+"/heartbeat", request, bearer(token))
	return err
}

func (c *Client) ReportInventory(ctx context.Context, agentID, token string, report agentv1.InventoryReport) error {
	_, err := call[json.RawMessage](ctx, c, http.MethodPut, "/api/v1/agents/"+url.PathEscape(agentID)+"/inventory", report, bearer(token))
	return err
}

func (c *Client) PollCommands(ctx context.Context, agentID, token string, limit int) ([]agentv1.Command, error) {
	path := fmt.Sprintf("/api/v1/agents/%s/commands?limit=%d", url.PathEscape(agentID), limit)
	return call[[]agentv1.Command](ctx, c, http.MethodGet, path, nil, bearer(token))
}

func (c *Client) AckCommand(ctx context.Context, agentID, token, commandID string, ack agentv1.CommandAckRequest) error {
	path := "/api/v1/agents/" + url.PathEscape(agentID) + "/commands/" + url.PathEscape(commandID) + "/ack"
	_, err := call[json.RawMessage](ctx, c, http.MethodPost, path, ack, bearer(token))
	return err
}

func call[T any](ctx context.Context, client *Client, method, path string, body any, headers map[string]string) (T, error) {
	var zero T
	var reader io.Reader
	if body != nil {
		content, err := json.Marshal(body)
		if err != nil {
			return zero, err
		}
		reader = bytes.NewReader(content)
	}
	requestURL := *client.baseURL
	requestURL.Path = strings.TrimRight(client.baseURL.Path, "/") + strings.SplitN(path, "?", 2)[0]
	if queryIndex := strings.IndexByte(path, '?'); queryIndex >= 0 {
		requestURL.RawQuery = path[queryIndex+1:]
	}
	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), reader)
	if err != nil {
		return zero, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "aiwm-agent/0.2")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return zero, err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return zero, err
	}
	var envelope agentv1.Response[T]
	if len(content) > 0 {
		if err := json.Unmarshal(content, &envelope); err != nil {
			return zero, fmt.Errorf("decode Control Plane response (%d): %w", response.StatusCode, err)
		}
	}
	if response.StatusCode == http.StatusUnauthorized {
		return zero, agent.ErrUnauthorized
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if envelope.Error != nil {
			return zero, fmt.Errorf("Control Plane %s: %s", envelope.Error.Code, envelope.Error.Message)
		}
		return zero, fmt.Errorf("Control Plane returned HTTP %d", response.StatusCode)
	}
	return envelope.Data, nil
}

func bearer(token string) map[string]string {
	return map[string]string{agentv1.AuthorizationHeader: "Bearer " + token}
}

func loadTLSConfig(files TLSConfig) (*tls.Config, error) {
	configuration := &tls.Config{MinVersion: tls.VersionTLS12}
	if files.CAFile != "" {
		content, err := os.ReadFile(files.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read agent CA file: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(content) {
			return nil, fmt.Errorf("agent CA file contains no valid PEM certificate")
		}
		configuration.RootCAs = pool
	}
	if (files.CertFile == "") != (files.KeyFile == "") {
		return nil, fmt.Errorf("both client certificate and key are required for mTLS")
	}
	if files.CertFile != "" {
		certificate, err := tls.LoadX509KeyPair(files.CertFile, files.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load agent client certificate: %w", err)
		}
		configuration.Certificates = []tls.Certificate{certificate}
	}
	return configuration, nil
}
