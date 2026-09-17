package livebrokers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
)

func TestBrokerErrFromStatusKiteMessage(t *testing.T) {
	resp := httptest.NewRecorder()
	resp.WriteHeader(http.StatusBadRequest)
	_, _ = resp.WriteString(`{"status":"error","message":"Insufficient funds for this order"}`)
	err := brokerErrFromStatus(resp.Result())
	if brokererr.CodeOf(err) != brokererr.CodeInsufficientMargin {
		t.Fatalf("code=%q", brokererr.CodeOf(err))
	}
}

func TestBrokerErrNoPositionMetaApi(t *testing.T) {
	resp := httptest.NewRecorder()
	resp.WriteHeader(http.StatusBadRequest)
	_, _ = resp.WriteString(`{"message":"Position with the specified id has already been closed"}`)
	err := brokerErrNoPosition(resp.Result())
	if brokererr.CodeOf(err) != brokererr.CodeNoPosition {
		t.Fatalf("code=%q", brokererr.CodeOf(err))
	}
}

func TestMapHTTPErrorCodeAuth(t *testing.T) {
	code := mapHTTPErrorCode(http.StatusUnauthorized, "")
	if code != brokererr.CodeAuthFailed {
		t.Fatalf("code=%q", code)
	}
}
