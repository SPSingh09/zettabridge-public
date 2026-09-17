package httpclient

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
)

// Client wraps outbound broker HTTP with timeouts, safe logging, and GET retries.
type Client struct {
	inner   *http.Client
	retries int
}

// New builds a broker HTTP client. getRetries applies only to idempotent methods (GET/HEAD).
func New(timeout time.Duration, getRetries int) *Client {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if getRetries < 0 {
		getRetries = 0
	}
	return &Client{
		inner:   &http.Client{Timeout: timeout},
		retries: getRetries,
	}
}

// Do executes the request, logging broker/op/method/path/status/latency without secrets.
// POST/PUT/DELETE are never retried.
func (c *Client) Do(ctx context.Context, broker, operation string, req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, http.ErrMissingFile
	}
	req = req.WithContext(ctx)

	method := req.Method
	path := sanitizePath(req.URL)
	maxAttempts := 1
	if isIdempotent(method) {
		maxAttempts = 1 + c.retries
	}

	start := time.Now()
	var (
		resp *http.Response
		err  error
	)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(retryBackoff(attempt))
			if req.GetBody != nil {
				body, bodyErr := req.GetBody()
				if bodyErr != nil {
					return nil, bodyErr
				}
				req.Body = body
			}
		}

		resp, err = c.inner.Do(req)
		if err != nil || !shouldRetry(method, resp) || attempt == maxAttempts-1 {
			break
		}
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}

	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	elapsed := time.Since(start)
	metrics.ObserveBrokerHTTP(broker, operation, elapsed)
	log.Printf(
		"broker_http: broker=%s op=%s method=%s path=%s status=%d latency_ms=%d ok=%t",
		broker, operation, method, path, status, elapsed.Milliseconds(), err == nil,
	)
	return resp, err
}

func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

func shouldRetry(method string, resp *http.Response) bool {
	if !isIdempotent(method) || resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func retryBackoff(attempt int) time.Duration {
	return time.Duration(attempt) * 150 * time.Millisecond
}

func sanitizePath(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Path
}

// RedactHeaderValue returns true for header names that must not appear in logs.
func RedactHeaderValue(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "authorization", "auth-token", "access-token", "x-api-key", "api-key":
		return true
	default:
		return strings.Contains(n, "token") || strings.Contains(n, "secret")
	}
}
