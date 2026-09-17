package livebrokers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
)

func httpNewJSONRequest(ctx context.Context, method, urlStr string, body []byte) (*http.Request, error) {
	var payload io.Reader
	if len(body) > 0 {
		payload = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, payload)
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	return req, nil
}

func httpNewFormRequest(ctx context.Context, method, urlStr string, form url.Values) (*http.Request, error) {
	encoded := form.Encode()
	req, err := http.NewRequestWithContext(ctx, method, urlStr, strings.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(encoded)), nil
	}
	return req, nil
}

func decodeJSON(resp *http.Response, dest any) error {
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	return nil
}

func brokerErrFromStatus(resp *http.Response) error {
	if resp == nil {
		return brokererr.New(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable))
	}
	body, _ := readResponseBody(resp)
	msg, errorType := extractKiteError(body)
	code := mapKiteErrorCode(resp.StatusCode, msg, errorType)
	if msg != "" {
		return brokererr.New(code, msg)
	}
	return brokererr.New(code, brokererr.PublicMessage(code))
}

func brokerErrNoPosition(resp *http.Response) error {
	if resp == nil {
		return brokererr.New(brokererr.CodeNoPosition, brokererr.PublicMessage(brokererr.CodeNoPosition))
	}
	body, _ := readResponseBody(resp)
	msg := extractErrorMessage(body)
	if resp.StatusCode == http.StatusNotFound || mentionsNoPosition(msg) {
		return brokererr.New(brokererr.CodeNoPosition, brokererr.PublicMessage(brokererr.CodeNoPosition))
	}
	return brokerErrFromStatus(resp)
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(data))
	return data, err
}

func extractErrorMessage(body []byte) string {
	msg, _ := extractKiteError(body)
	return msg
}

// extractKiteError parses a broker error body, returning both the
// human-readable message and (when present) Kite Connect's error_type field
// ("TokenException", "PermissionException", "InputException", etc). Other
// brokers' response bodies simply don't have error_type, so errorType comes
// back empty for them and callers fall back to status/message heuristics.
func extractKiteError(body []byte) (message, errorType string) {
	if len(body) == 0 {
		return "", ""
	}
	var kite struct {
		Message   string `json:"message"`
		ErrorType string `json:"error_type"`
	}
	if json.Unmarshal(body, &kite) == nil {
		errorType = strings.TrimSpace(kite.ErrorType)
		if strings.TrimSpace(kite.Message) != "" {
			return strings.TrimSpace(kite.Message), errorType
		}
	}
	var meta struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if json.Unmarshal(body, &meta) == nil {
		if s := strings.TrimSpace(meta.Message); s != "" {
			return s, errorType
		}
		if s := strings.TrimSpace(meta.Error); s != "" {
			return s, errorType
		}
	}
	return strings.TrimSpace(string(body)), errorType
}

// mapKiteErrorCode is mapHTTPErrorCode plus Kite Connect's error_type field.
// Kite returns both a real auth/session failure (error_type=TokenException)
// and an app-permission-scope problem (error_type=PermissionException, e.g.
// the market-data/quote API requiring a separate subscription from order
// placement) as the same HTTP 401/403 — status code alone can't tell them
// apart, so error_type is checked first when the broker provides one.
func mapKiteErrorCode(status int, msg, errorType string) brokererr.Code {
	switch errorType {
	case "TokenException":
		return brokererr.CodeAuthFailed
	case "PermissionException":
		return brokererr.CodePermissionDenied
	}
	return mapHTTPErrorCode(status, msg)
}

func mapHTTPErrorCode(status int, msg string) brokererr.Code {
	lower := strings.ToLower(msg)
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return brokererr.CodeAuthFailed
	case http.StatusTooManyRequests:
		return brokererr.CodeRateLimited
	case http.StatusNotFound:
		if mentionsNoPosition(msg) {
			return brokererr.CodeNoPosition
		}
		return brokererr.CodeInvalidSymbol
	}
	if strings.Contains(lower, "margin") || strings.Contains(lower, "insufficient") {
		return brokererr.CodeInsufficientMargin
	}
	if strings.Contains(lower, "market") && strings.Contains(lower, "closed") {
		return brokererr.CodeMarketClosed
	}
	if strings.Contains(lower, "symbol") || strings.Contains(lower, "instrument") {
		return brokererr.CodeInvalidSymbol
	}
	if mentionsNoPosition(msg) {
		return brokererr.CodeNoPosition
	}
	return brokererr.CodeInternal
}

func mentionsNoPosition(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "no position") ||
		strings.Contains(lower, "position closed") ||
		strings.Contains(lower, "already been closed") ||
		strings.Contains(lower, "position not found") ||
		strings.Contains(lower, "no open position")
}

func isSuccessStatus(code int) bool {
	return code == http.StatusOK || code == http.StatusCreated
}
