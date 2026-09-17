package livebrokers

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/symboltoken"
)

// DhanBroker is a DhanHQ v2 adapter (live HTTP, market orders only in v1).
type DhanBroker struct {
	adapterBase
	parsed brokercreds.Parsed
}

func (b *DhanBroker) PlaceOrder(ctx context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	if req.Action == "CLOSE" {
		return b.squareOff(ctx, req)
	}
	return b.placeMarketOrder(ctx, indianTxnType(req.Action), req)
}

func (b *DhanBroker) placeMarketOrder(ctx context.Context, txnType string, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	securityID, tradingSymbol, err := b.resolveSecurity(ctx, req.Exchange, req.Symbol)
	if err != nil {
		return nil, err
	}
	return b.placeMarketOrderWithSecurity(ctx, txnType, req, securityID, tradingSymbol)
}

func (b *DhanBroker) placeMarketOrderWithSecurity(ctx context.Context, txnType string, req *domain.PlaceRequest, securityID, tradingSymbol string) (*domain.OrderResult, error) {
	if req.OrderType == domain.OrderBracket {
		return nil, brokererr.New(brokererr.CodeBracketNotSupported,
			brokererr.PublicMessage(brokererr.CodeBracketNotSupported))
	}
	segment := dhanExchangeSegment(req.Exchange)
	payload := map[string]interface{}{
		"dhanClientId":    b.parsed.ClientID,
		"transactionType": txnType,
		"exchangeSegment": segment,
		"productType":     dhanProductType(req.Product),
		"orderType":       "MARKET",
		"validity":        "DAY",
		"securityId":      securityID,
		"quantity":        int(req.Quantity),
	}
	// correlationId carries the SEBI algo ID for Dhan (confirmed per Dhan support).
	// Max 30 alphanumeric chars; see internal/algo/algo.go brokerMaxTagLen.
	if tag := strings.TrimSpace(req.AlgoID); tag != "" {
		payload["correlationId"] = tag
	}
	_ = tradingSymbol

	body, _ := json.Marshal(payload)
	url := b.baseURL() + dhanPathOrders
	httpReq, err := httpNewJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setDhanHeaders(httpReq)

	op := "place_order"
	if req.Action == "CLOSE" {
		op = "square_off"
	}
	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, op, httpReq)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}

	var result struct {
		OrderID     string `json:"orderId"`
		OrderStatus string `json:"orderStatus"`
	}
	if err := decodeDhanJSON(resp, &result); err != nil {
		return nil, err
	}
	prefix := "DHAN"
	if req.Action == "CLOSE" {
		prefix = "DHAN-CLOSE"
	}
	out := submittedResult(prefix)
	out.OrderID = result.OrderID
	return out, nil
}

func (b *DhanBroker) squareOff(ctx context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	netQty, securityID, tradingSymbol, err := b.netPositionQty(ctx, req)
	if err != nil {
		return nil, err
	}
	if netQty == 0 {
		return nil, brokererr.New(brokererr.CodeNoPosition, brokererr.PublicMessage(brokererr.CodeNoPosition))
	}

	txnType := "SELL"
	qty := netQty
	if netQty < 0 {
		txnType = "BUY"
		qty = -netQty
	}

	closeReq := *req
	closeReq.Action = "CLOSE"
	closeReq.Quantity = float64(qty)
	return b.placeMarketOrderWithSecurity(ctx, txnType, &closeReq, securityID, tradingSymbol)
}

type dhanPosition struct {
	TradingSymbol   string `json:"tradingSymbol"`
	SecurityID      string `json:"securityId"`
	ExchangeSegment string `json:"exchangeSegment"`
	ProductType     string `json:"productType"`
	NetQty          int    `json:"netQty"`
}

func (b *DhanBroker) netPositionQty(ctx context.Context, req *domain.PlaceRequest) (netQty int, securityID, tradingSymbol string, err error) {
	secID, ts, err := b.resolveSecurity(ctx, req.Exchange, req.Symbol)
	if err != nil {
		return 0, "", "", err
	}

	url := b.baseURL() + dhanPathPositions
	httpReq, err := httpNewJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", "", err
	}
	b.setDhanHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "get_positions", httpReq)
	if err != nil {
		return 0, "", "", brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}

	var positions []dhanPosition
	if err := decodeDhanBody(resp, &positions); err != nil {
		return 0, "", "", err
	}

	wantSegment := dhanExchangeSegment(req.Exchange)
	wantProduct := dhanProductType(req.Product)

	for _, p := range positions {
		if !strings.EqualFold(strings.TrimSpace(p.TradingSymbol), ts) &&
			!strings.EqualFold(strings.TrimSpace(p.TradingSymbol), strings.TrimSpace(req.Symbol)) {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(p.ExchangeSegment)) != wantSegment {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(p.ProductType), wantProduct) {
			continue
		}
		st := p.SecurityID
		if st == "" {
			st = secID
		}
		sym := p.TradingSymbol
		if sym == "" {
			sym = ts
		}
		return p.NetQty, st, sym, nil
	}
	return 0, secID, ts, nil
}

func (b *DhanBroker) CancelOrder(ctx context.Context, req *domain.CancelRequest) error {
	url := b.baseURL() + dhanPathCancelOrder + req.OrderID
	httpReq, err := httpNewJSONRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setDhanHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "cancel_order", httpReq)
	if err != nil {
		return brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	// Dhan DELETE returns 202 Accepted on success; treat any 2xx as success.
	if resp.StatusCode == http.StatusNotFound {
		return brokererr.New(brokererr.CodeOrderNotFound, brokererr.PublicMessage(brokererr.CodeOrderNotFound))
	}
	if resp.StatusCode >= 400 {
		body, _ := readResponseBody(resp)
		if env, ok := parseDhanFailure(body); ok {
			return env.toError(resp.StatusCode)
		}
		return brokerErrFromStatus(resp)
	}
	resp.Body.Close()
	return nil
}

func (b *DhanBroker) GetAccountEquity(ctx context.Context) (float64, error) {
	url := b.baseURL() + dhanPathFundLimit
	httpReq, err := httpNewJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	b.setDhanHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "get_equity", httpReq)
	if err != nil {
		return 0, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}

	var result struct {
		AvailableBalance    float64 `json:"availabelBalance"`
		AvailableBalanceAlt float64 `json:"availableBalance"`
	}
	if err := decodeDhanJSON(resp, &result); err != nil {
		return 0, err
	}
	equity := result.AvailableBalance
	if equity <= 0 {
		equity = result.AvailableBalanceAlt
	}
	if equity <= 0 {
		return 0, brokererr.New(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal))
	}
	return equity, nil
}

func (b *DhanBroker) resolveSecurity(ctx context.Context, exchange, symbol string) (securityID, tradingSymbol string, err error) {
	if b.infra == nil || b.infra.SymbolTokens == nil {
		return b.lookupSecurity(ctx, exchange, symbol)
	}
	securityID, tradingSymbol, err = b.infra.SymbolTokens.Resolve(ctx, "dhan", exchange, symbol, b.lookupSecurity)
	if err != nil {
		if err == symboltoken.ErrNotFound {
			return "", "", brokererr.New(brokererr.CodeInvalidSymbol, brokererr.PublicMessage(brokererr.CodeInvalidSymbol))
		}
		return "", "", err
	}
	return securityID, tradingSymbol, nil
}

func (b *DhanBroker) lookupSecurity(ctx context.Context, exchange, symbol string) (securityID, tradingSymbol string, err error) {
	segment := dhanExchangeSegment(exchange)
	url := b.baseURL() + dhanPathInstrument + segment
	httpReq, err := httpNewJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	b.setDhanHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "instrument_list", httpReq)
	if err != nil {
		return "", "", brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", "", brokerErrFromStatus(resp)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if env, ok := parseDhanFailure(body); ok {
			return "", "", env.toError(resp.StatusCode)
		}
		return "", "", brokerErrFromStatus(resp)
	}

	id, ts, ok := findSecurityInInstrumentCSV(resp.Body, symbol)
	if !ok {
		return "", "", symboltoken.ErrNotFound
	}
	return id, ts, nil
}

func (b *DhanBroker) setDhanHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("access-token", b.parsed.AccessToken)
}

func dhanExchangeSegment(exchange string) string {
	switch strings.ToUpper(strings.TrimSpace(exchange)) {
	case "BSE":
		return "BSE_EQ"
	default:
		return "NSE_EQ"
	}
}

func dhanProductType(product string) string {
	switch strings.ToUpper(strings.TrimSpace(product)) {
	case "MIS":
		return "INTRADAY"
	case "CNC":
		return "CNC"
	case "NRML":
		return "MARGIN"
	default:
		return "INTRADAY"
	}
}

type dhanEnvelope struct {
	Status       string `json:"status"`
	ErrorType    string `json:"errorType"`
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}

func parseDhanFailure(body []byte) (dhanEnvelope, bool) {
	var env dhanEnvelope
	if len(body) == 0 {
		return env, false
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return env, false
	}
	return env, strings.EqualFold(env.Status, "failure")
}

func (e dhanEnvelope) toError(httpStatus int) error {
	msg := strings.TrimSpace(e.ErrorMessage)
	code := mapDhanErrorCode(httpStatus, e.ErrorType, e.ErrorCode, msg)
	if msg == "" {
		msg = brokererr.PublicMessage(code)
	}
	return brokererr.New(code, msg)
}

func mapDhanErrorCode(httpStatus int, errorType, errorCode, msg string) brokererr.Code {
	lower := strings.ToLower(msg + " " + errorType + " " + errorCode)
	switch {
	case httpStatus == http.StatusUnauthorized || httpStatus == http.StatusForbidden || strings.Contains(lower, "auth"):
		return brokererr.CodeAuthFailed
	case strings.Contains(lower, "rate") || errorCode == "RL001":
		return brokererr.CodeRateLimited
	case strings.Contains(lower, "margin") || strings.Contains(lower, "insufficient"):
		return brokererr.CodeInsufficientMargin
	case strings.Contains(lower, "position"):
		return brokererr.CodeNoPosition
	case strings.Contains(lower, "security") || strings.Contains(lower, "symbol"):
		return brokererr.CodeInvalidSymbol
	default:
		return brokererr.CodeInternal
	}
}

func decodeDhanJSON(resp *http.Response, dest any) error {
	body, err := readResponseBody(resp)
	if err != nil {
		return brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	if env, ok := parseDhanFailure(body); ok {
		return env.toError(resp.StatusCode)
	}
	if !isSuccessStatus(resp.StatusCode) {
		return brokerErrFromStatus(resp)
	}
	if dest != nil && len(body) > 0 {
		if err := json.Unmarshal(body, dest); err != nil {
			return brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
		}
	}
	return nil
}

func decodeDhanBody(resp *http.Response, dest any) error {
	body, err := readResponseBody(resp)
	if err != nil {
		return brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	if env, ok := parseDhanFailure(body); ok {
		return env.toError(resp.StatusCode)
	}
	if !isSuccessStatus(resp.StatusCode) {
		return brokerErrFromStatus(resp)
	}
	if dest != nil && len(body) > 0 {
		if err := json.Unmarshal(body, dest); err != nil {
			return brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
		}
	}
	return nil
}
