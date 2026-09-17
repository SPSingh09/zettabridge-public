package livebrokers

import (
	"context"
	"net/http"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
)

// KiteOrderSnapshot is the latest known state of a Kite order.
type KiteOrderSnapshot struct {
	OrderID       string
	Status        string
	AveragePrice  float64
	StatusMessage string
}

type kiteOrderRow struct {
	OrderID       string  `json:"order_id"`
	Status        string  `json:"status"`
	AveragePrice  float64 `json:"average_price"`
	StatusMessage string  `json:"status_message"`
}

// FetchTodayOrders returns today's order book keyed by Kite order_id.
func (b *ZerodhaBroker) FetchTodayOrders(ctx context.Context) (map[string]KiteOrderSnapshot, error) {
	url := b.baseURL() + "/orders"
	httpReq, err := httpNewJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setKiteHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "get_orders", httpReq)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, brokerErrFromStatus(resp)
	}

	var result struct {
		Data []kiteOrderRow `json:"data"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		return nil, err
	}

	out := make(map[string]KiteOrderSnapshot, len(result.Data))
	for _, row := range result.Data {
		if row.OrderID == "" {
			continue
		}
		out[row.OrderID] = KiteOrderSnapshot{
			OrderID:       row.OrderID,
			Status:        row.Status,
			AveragePrice:  row.AveragePrice,
			StatusMessage: row.StatusMessage,
		}
	}
	return out, nil
}

// FetchOrderHistory returns the latest state for a single order via GET /orders/:order_id.
func (b *ZerodhaBroker) FetchOrderHistory(ctx context.Context, orderID string) (*KiteOrderSnapshot, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, brokererr.New(brokererr.CodeInternal, "order id required")
	}
	url := b.baseURL() + "/orders/" + orderID
	httpReq, err := httpNewJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setKiteHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "get_order_history", httpReq)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, brokererr.New(brokererr.CodeOrderNotFound, brokererr.PublicMessage(brokererr.CodeOrderNotFound))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, brokerErrFromStatus(resp)
	}

	var result struct {
		Data []kiteOrderRow `json:"data"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	if len(result.Data) == 0 {
		return nil, brokererr.New(brokererr.CodeOrderNotFound, brokererr.PublicMessage(brokererr.CodeOrderNotFound))
	}
	row := result.Data[len(result.Data)-1]
	return &KiteOrderSnapshot{
		OrderID:       row.OrderID,
		Status:        row.Status,
		AveragePrice:  row.AveragePrice,
		StatusMessage: row.StatusMessage,
	}, nil
}

// MapKiteStatus maps a Kite order status to a ZettaBridge trade status.
// The second return is true when the order reached a terminal state.
func MapKiteStatus(kiteStatus string) (tradeStatus string, terminal bool) {
	switch strings.ToUpper(strings.TrimSpace(kiteStatus)) {
	case "COMPLETE":
		return domain.StatusFilled, true
	case "REJECTED":
		return domain.StatusRejected, true
	case "CANCELLED":
		return "cancelled", true
	default:
		return domain.StatusSubmitted, false
	}
}

// ApplyKiteSnapshot returns trade updates when the Kite order reached a terminal state.
func ApplyKiteSnapshot(snap KiteOrderSnapshot) (newStatus string, fillPrice float64, errMsg string, ok bool) {
	newStatus, terminal := MapKiteStatus(snap.Status)
	if !terminal {
		return "", 0, "", false
	}
	fillPrice = snap.AveragePrice
	if newStatus == domain.StatusRejected && strings.TrimSpace(snap.StatusMessage) != "" {
		errMsg = snap.StatusMessage
	}
	return newStatus, fillPrice, errMsg, true
}
