package integration

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokerfactory"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/platform/queue"
	"github.com/SPSingh09/zettabridge/internal/store"
)

func TestLiveQueueZerodhaAuthFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/orders/regular" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"status":"error","message":"Invalid api_key or access_token"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	fs := &memStore{
		user: &store.User{ID: "u1", Plan: plan.PlanPro},
		cred: &store.BrokerCredential{
			ID:             "c1",
			BrokerType:     "zerodha",
			EncryptedCreds: "apikey:badtoken",
			AccountMode:    plan.AccountLive,
			Exchange:       "NSE",
			Product:        "MIS",
		},
	}
	q := queue.NewForTest(fs, &memRateLimiter{count: 1}, testCredKey(), brokerfactory.ModeLive, liveInfra(srv.URL, ""))
	q.ProcessJobSync(baseJob("u1", "c1", "RELIANCE"))

	tr := fs.lastTrade()
	if tr == nil {
		t.Fatal("expected trade")
	}
	if tr.Status != domain.StatusRejected {
		t.Fatalf("status=%q want rejected", tr.Status)
	}
	if tr.ErrorCode != string(brokererr.CodeAuthFailed) {
		t.Fatalf("error_code=%q", tr.ErrorCode)
	}
}
