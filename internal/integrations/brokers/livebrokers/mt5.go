package livebrokers

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/brokercreds"
)

// MT5Broker is a MetaApi.cloud adapter (live HTTP).
type MT5Broker struct {
	adapterBase
	parsed brokercreds.Parsed
}

func (b *MT5Broker) PlaceOrder(ctx context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	accountID := b.parsed.AccountID
	url := fmt.Sprintf("%s/users/current/accounts/%s/trade", b.baseURL(), accountID)

	payload := map[string]interface{}{
		"symbol":  req.Symbol,
		"comment": req.Comment,
	}
	switch req.Action {
	case "CLOSE":
		payload["actionType"] = "POSITIONS_CLOSE_SYMBOL"
	default:
		payload["actionType"] = mt5ActionType(req.Action)
		payload["volume"] = req.Quantity
	}

	var slPrice, tpPrice float64
	if req.OrderType == domain.OrderBracket && req.Action != "CLOSE" {
		price, err := b.getCurrentPrice(ctx, req.Symbol, req.Action)
		if err != nil {
			return nil, err
		}
		isSell := strings.ToUpper(req.Action) == "SELL"
		if req.SLPoints > 0 {
			if isSell {
				slPrice = price + float64(req.SLPoints)*0.0001
			} else {
				slPrice = price - float64(req.SLPoints)*0.0001
			}
			payload["stopLoss"] = slPrice
		}
		if req.TPPoints > 0 {
			if isSell {
				tpPrice = price - float64(req.TPPoints)*0.0001
			} else {
				tpPrice = price + float64(req.TPPoints)*0.0001
			}
			payload["takeProfit"] = tpPrice
		}
	}

	body, _ := json.Marshal(payload)
	httpReq, err := httpNewJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	httpReq.Header.Set("auth-token", b.parsed.AuthToken)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "place_order", httpReq)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	if !isSuccessStatus(resp.StatusCode) {
		if req.Action == "CLOSE" {
			return nil, brokerErrNoPosition(resp)
		}
		return nil, brokerErrFromStatus(resp)
	}

	var result struct {
		OrderID string `json:"orderId"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	out := submittedResult("MT5")
	out.OrderID = result.OrderID
	out.SLPrice = slPrice
	out.TPPrice = tpPrice
	return out, nil
}

func (b *MT5Broker) getCurrentPrice(ctx context.Context, symbol, action string) (float64, error) {
	url := fmt.Sprintf("%s/users/current/accounts/%s/symbols/%s/current-price",
		b.baseURL(), b.parsed.AccountID, symbol)
	httpReq, err := httpNewJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	httpReq.Header.Set("auth-token", b.parsed.AuthToken)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "get_price", httpReq)
	if err != nil {
		return 0, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	if !isSuccessStatus(resp.StatusCode) {
		return 0, brokerErrFromStatus(resp)
	}

	var result struct {
		Ask float64 `json:"ask"`
		Bid float64 `json:"bid"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		return 0, err
	}
	if strings.ToUpper(action) == "SELL" {
		if result.Bid <= 0 {
			return 0, brokererr.New(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal))
		}
		return result.Bid, nil
	}
	if result.Ask <= 0 {
		return 0, brokererr.New(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal))
	}
	return result.Ask, nil
}

func (b *MT5Broker) CancelOrder(ctx context.Context, req *domain.CancelRequest) error {
	accountID := b.parsed.AccountID
	url := fmt.Sprintf("%s/users/current/accounts/%s/trade", b.baseURL(), accountID)

	payload := map[string]interface{}{
		"actionType": "ORDER_CANCEL",
		"orderId":    req.OrderID,
	}
	body, _ := json.Marshal(payload)
	httpReq, err := httpNewJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	httpReq.Header.Set("auth-token", b.parsed.AuthToken)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "cancel_order", httpReq)
	if err != nil {
		return brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	if resp.StatusCode == http.StatusNotFound {
		return brokererr.New(brokererr.CodeOrderNotFound, brokererr.PublicMessage(brokererr.CodeOrderNotFound))
	}
	if !isSuccessStatus(resp.StatusCode) {
		return brokerErrFromStatus(resp)
	}
	resp.Body.Close()
	return nil
}

func (b *MT5Broker) GetAccountEquity(ctx context.Context) (float64, error) {
	url := fmt.Sprintf("%s/users/current/accounts/%s/account-information", b.baseURL(), b.parsed.AccountID)

	httpReq, err := httpNewJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	httpReq.Header.Set("auth-token", b.parsed.AuthToken)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "get_equity", httpReq)
	if err != nil {
		return 0, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, brokerErrFromStatus(resp)
	}

	var result struct {
		Equity float64 `json:"equity"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		return 0, err
	}
	if result.Equity <= 0 {
		return 0, brokererr.New(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal))
	}
	return result.Equity, nil
}

func mt5ActionType(action string) string {
	switch strings.ToUpper(action) {
	case "BUY":
		return "ORDER_TYPE_BUY"
	case "SELL":
		return "ORDER_TYPE_SELL"
	default:
		return "ORDER_TYPE_BUY"
	}
}
