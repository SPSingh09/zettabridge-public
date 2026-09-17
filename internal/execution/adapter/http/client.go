package httpadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/SPSingh09/zettabridge/internal/execution"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
)

const serviceTokenHeader = "X-ZB-Service-Token"

// Client calls a remote execution adapter over HTTP.
type Client struct {
	BaseURL      string
	ServiceToken string
	HTTP         *http.Client
}

func NewClient(baseURL, serviceToken string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &Client{
		BaseURL:      strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		ServiceToken: strings.TrimSpace(serviceToken),
		HTTP:         &http.Client{Timeout: timeout},
	}
}

// PlaceOrder POSTs an execution command to the adapter.
func (c *Client) PlaceOrder(ctx context.Context, cmd execution.ExecutionCommand) (execution.ExecutionOutcome, error) {
	var out execution.ExecutionOutcome
	if c == nil || c.BaseURL == "" {
		return out, fmt.Errorf("adapter URL not configured")
	}
	broker := strings.TrimSpace(cmd.Destination.Broker)
	if broker == "" {
		broker = "unknown"
	}
	start := time.Now()
	defer func() {
		metrics.ObserveBrokerHTTP(broker, "place_order", time.Since(start))
	}()

	body, err := json.Marshal(cmd)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/orders", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.ServiceToken != "" {
		req.Header.Set(serviceTokenHeader, c.ServiceToken)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		log.Printf("adapter http: place order failed: %v", err)
		return out, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return out, err
	}
	if resp.StatusCode >= 400 {
		log.Printf("adapter http: place order status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		return out, brokererr.New(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal))
	}
	var envelope struct {
		Data execution.ExecutionOutcome `json:"data"`
	}
	if err := json.Unmarshal(respBody, &envelope); err != nil {
		return out, fmt.Errorf("adapter response parse: %w", err)
	}
	return envelope.Data, nil
}
