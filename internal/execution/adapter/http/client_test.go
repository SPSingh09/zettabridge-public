package httpadapter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/SPSingh09/zettabridge/internal/execution"
	httpadapter "github.com/SPSingh09/zettabridge/internal/execution/adapter/http"
	"github.com/SPSingh09/zettabridge/internal/platform/metrics"
)

func TestClientPlaceOrder(t *testing.T) {
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/orders" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		gotToken = r.Header.Get("X-ZB-Service-Token")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": execution.ExecutionOutcome{
				RequestID:     "trade-1",
				Status:        "submitted",
				BrokerOrderID: "MOCK-1",
			},
		})
	}))
	defer srv.Close()

	client := httpadapter.NewClient(srv.URL, "svc-token", 0)
	out, err := client.PlaceOrder(context.Background(), execution.ExecutionCommand{
		RequestID: "trade-1",
		Tenant:    execution.Tenant{UserID: "u1", Plan: "pro"},
		Destination: execution.Destination{
			Kind:         "live_broker",
			Broker:       "zerodha",
			CredentialID: "c1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.BrokerOrderID != "MOCK-1" || gotToken != "svc-token" {
		t.Fatalf("order=%q token=%q", out.BrokerOrderID, gotToken)
	}
	if testutil.CollectAndCount(metrics.BrokerHTTPDuration) == 0 {
		t.Fatal("expected broker HTTP histogram sample after adapter PlaceOrder")
	}
}
