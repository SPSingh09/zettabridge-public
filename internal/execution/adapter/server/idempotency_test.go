package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/SPSingh09/zettabridge/internal/execution"
	"github.com/SPSingh09/zettabridge/internal/execution/adapter/server"
)

type memIdempotency struct {
	mu   sync.Mutex
	data map[string]execution.ExecutionOutcome
}

func (m *memIdempotency) GetAdapterOrderOutcome(_ context.Context, key string) (*execution.ExecutionOutcome, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out, ok := m.data[key]
	if !ok {
		return nil, false, nil
	}
	copy := out
	return &copy, true, nil
}

func (m *memIdempotency) PutAdapterOrderOutcome(_ context.Context, key string, out execution.ExecutionOutcome, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = map[string]execution.ExecutionOutcome{}
	}
	m.data[key] = out
	return nil
}

type countingExecutor struct {
	calls int
}

func (c *countingExecutor) PlaceOrder(_ context.Context, cmd execution.ExecutionCommand) (execution.ExecutionOutcome, error) {
	c.calls++
	return execution.ExecutionOutcome{
		RequestID:     cmd.RequestID,
		Status:        "submitted",
		BrokerOrderID: "ORD-1",
	}, nil
}

func (c *countingExecutor) CancelOrder(context.Context, execution.CancelCommand) (execution.ExecutionOutcome, error) {
	return execution.ExecutionOutcome{}, nil
}

func TestAdapterIdempotencyReturnsCachedOutcome(t *testing.T) {
	inner := &countingExecutor{}
	app := server.NewApp(server.Config{
		Name:         "test",
		Executor:     inner,
		ServiceToken: "token",
		Idempotency:  &memIdempotency{},
	})

	body, _ := json.Marshal(execution.ExecutionCommand{
		RequestID:      "trade-1",
		IdempotencyKey: "idem-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ZB-Service-Token", "token")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}

	resp2, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp2.StatusCode)
	}
	if inner.calls != 1 {
		t.Fatalf("expected 1 executor call, got %d", inner.calls)
	}
}
