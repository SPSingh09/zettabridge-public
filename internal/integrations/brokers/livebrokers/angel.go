package livebrokers

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/symboltoken"
)

// AngelBroker is an Angel One SmartAPI adapter (live HTTP, market orders only in v1).
type AngelBroker struct {
	adapterBase
	parsed brokercreds.Parsed
}

func (b *AngelBroker) PlaceOrder(ctx context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	if req.Action == "CLOSE" {
		return b.squareOff(ctx, req)
	}
	if req.OrderType == domain.OrderBracket {
		return b.placeROBOOrder(ctx, indianTxnType(req.Action), req)
	}
	return b.placeMarketOrder(ctx, indianTxnType(req.Action), req)
}

func (b *AngelBroker) placeMarketOrder(ctx context.Context, txnType string, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	token, tradingsymbol, err := b.resolveInstrument(ctx, req.Exchange, req.Symbol)
	if err != nil {
		return nil, err
	}

	payload := map[string]string{
		"variety":          "NORMAL",
		"tradingsymbol":    tradingsymbol,
		"symboltoken":      token,
		"transactiontype":  txnType,
		"exchange":         req.Exchange,
		"ordertype":        "MARKET",
		"producttype":      angelProductType(req.Product),
		"duration":         "DAY",
		"price":            "0",
		"squareoff":        "0",
		"stoploss":         "0",
		"quantity":         strconv.Itoa(int(req.Quantity)),
	}
	if tag := strings.TrimSpace(req.AlgoID); tag != "" {
		payload["ordertag"] = tag
	}

	body, _ := json.Marshal(payload)
	url := b.baseURL() + angelPathPlaceOrder
	httpReq, err := httpNewJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setAngelHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "place_order", httpReq)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}

	var result struct {
		Data struct {
			OrderID string `json:"orderid"`
		} `json:"data"`
	}
	if err := decodeAngelJSON(resp, &result); err != nil {
		return nil, err
	}
	out := submittedResult("ANGEL")
	out.OrderID = result.Data.OrderID
	return out, nil
}

func (b *AngelBroker) getLTP(ctx context.Context, exchange, symbolToken string) (float64, error) {
	payload := map[string]interface{}{
		"mode": "LTP",
		"exchangeTokens": map[string][]string{
			strings.ToUpper(exchange): {symbolToken},
		},
	}
	body, _ := json.Marshal(payload)
	url := b.baseURL() + angelPathLTP
	httpReq, err := httpNewJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return 0, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setAngelHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "get_ltp", httpReq)
	if err != nil {
		return 0, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}

	var result struct {
		Data struct {
			Fetched []struct {
				LTP float64 `json:"ltp"`
			} `json:"fetched"`
		} `json:"data"`
	}
	if err := decodeAngelJSON(resp, &result); err != nil {
		return 0, err
	}
	if len(result.Data.Fetched) == 0 || result.Data.Fetched[0].LTP <= 0 {
		return 0, brokererr.New(brokererr.CodeInvalidSymbol, brokererr.PublicMessage(brokererr.CodeInvalidSymbol))
	}
	return result.Data.Fetched[0].LTP, nil
}

func (b *AngelBroker) placeROBOOrder(ctx context.Context, txnType string, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	token, tradingsymbol, err := b.resolveInstrument(ctx, req.Exchange, req.Symbol)
	if err != nil {
		return nil, err
	}

	ltp, err := b.getLTP(ctx, req.Exchange, token)
	if err != nil {
		return nil, err
	}

	var slPrice, tpPrice float64
	if strings.ToUpper(txnType) == "SELL" {
		slPrice = ltp + float64(req.SLPoints)
		tpPrice = ltp - float64(req.TPPoints)
	} else {
		slPrice = ltp - float64(req.SLPoints)
		tpPrice = ltp + float64(req.TPPoints)
	}

	slAbs := math.Abs(slPrice - ltp)
	tpAbs := math.Abs(tpPrice - ltp)

	payload := map[string]string{
		"variety":         "ROBO",
		"tradingsymbol":   tradingsymbol,
		"symboltoken":     token,
		"transactiontype": txnType,
		"exchange":        req.Exchange,
		"ordertype":       "MARKET",
		"producttype":     angelProductType(req.Product),
		"duration":        "DAY",
		"price":           "0",
		"squareoff":       strconv.FormatFloat(tpAbs, 'f', 2, 64),
		"stoploss":        strconv.FormatFloat(slAbs, 'f', 2, 64),
		"quantity":        strconv.Itoa(int(req.Quantity)),
	}
	if tag := strings.TrimSpace(req.AlgoID); tag != "" {
		payload["ordertag"] = tag
	}

	body, _ := json.Marshal(payload)
	url := b.baseURL() + angelPathPlaceOrder
	httpReq, err := httpNewJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setAngelHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "place_robo", httpReq)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}

	var result struct {
		Data struct {
			OrderID string `json:"orderid"`
		} `json:"data"`
	}
	if err := decodeAngelJSON(resp, &result); err != nil {
		return nil, err
	}
	out := submittedResult("ANGEL-ROBO")
	out.OrderID = result.Data.OrderID
	out.SLPrice = slPrice
	if req.TPPoints > 0 {
		out.TPPrice = tpPrice
	}
	return out, nil
}

func (b *AngelBroker) squareOff(ctx context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	netQty, tradingsymbol, symbolToken, err := b.netPositionQty(ctx, req)
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
	closeReq.Symbol = tradingsymbol
	closeReq.Quantity = float64(qty)
	// Reuse resolved token via tradingsymbol on closeReq — resolveInstrument would search again;
	// pass token directly by placing with known token.
	return b.placeMarketOrderWithToken(ctx, txnType, &closeReq, symbolToken, tradingsymbol)
}

func (b *AngelBroker) placeMarketOrderWithToken(ctx context.Context, txnType string, req *domain.PlaceRequest, token, tradingsymbol string) (*domain.OrderResult, error) {
	payload := map[string]string{
		"variety":         "NORMAL",
		"tradingsymbol":   tradingsymbol,
		"symboltoken":     token,
		"transactiontype": txnType,
		"exchange":        req.Exchange,
		"ordertype":       "MARKET",
		"producttype":     angelProductType(req.Product),
		"duration":        "DAY",
		"price":           "0",
		"squareoff":       "0",
		"stoploss":        "0",
		"quantity":        strconv.Itoa(int(req.Quantity)),
	}
	if tag := strings.TrimSpace(req.AlgoID); tag != "" {
		payload["ordertag"] = tag
	}

	body, _ := json.Marshal(payload)
	url := b.baseURL() + angelPathPlaceOrder
	httpReq, err := httpNewJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setAngelHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "square_off", httpReq)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}

	var result struct {
		Data struct {
			OrderID string `json:"orderid"`
		} `json:"data"`
	}
	if err := decodeAngelJSON(resp, &result); err != nil {
		return nil, err
	}
	out := submittedResult("ANGEL-CLOSE")
	out.OrderID = result.Data.OrderID
	return out, nil
}

type angelPosition struct {
	TradingSymbol string          `json:"tradingsymbol"`
	Exchange      string          `json:"exchange"`
	ProductType   string          `json:"producttype"`
	NetQty        json.RawMessage `json:"netqty"`
	SymbolToken   string          `json:"symboltoken"`
}

func parseAngelNetQty(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return 0, false
		}
		return int(math.Round(f)), true
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return int(math.Round(f)), true
	}
	return 0, false
}

func (b *AngelBroker) netPositionQty(ctx context.Context, req *domain.PlaceRequest) (netQty int, tradingsymbol, symbolToken string, err error) {
	token, tradingsymbol, err := b.resolveInstrument(ctx, req.Exchange, req.Symbol)
	if err != nil {
		return 0, "", "", err
	}

	url := b.baseURL() + angelPathGetPosition
	httpReq, err := httpNewJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, "", "", err
	}
	b.setAngelHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "get_positions", httpReq)
	if err != nil {
		return 0, "", "", brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}

	var result struct {
		Data []angelPosition `json:"data"`
	}
	if err := decodeAngelJSON(resp, &result); err != nil {
		return 0, "", "", err
	}

	wantProduct := angelProductType(req.Product)
	wantExchange := strings.ToUpper(strings.TrimSpace(req.Exchange))

	for _, p := range result.Data {
		if !strings.EqualFold(p.TradingSymbol, tradingsymbol) {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(p.Exchange)) != wantExchange {
			continue
		}
		if !strings.EqualFold(p.ProductType, wantProduct) {
			continue
		}
		qty, ok := parseAngelNetQty(p.NetQty)
		if !ok {
			continue
		}
		st := p.SymbolToken
		if st == "" {
			st = token
		}
		return qty, tradingsymbol, st, nil
	}
	return 0, tradingsymbol, token, nil
}

func (b *AngelBroker) CancelOrder(ctx context.Context, req *domain.CancelRequest) error {
	variety := req.Variety
	if variety == "" {
		variety = "NORMAL"
	}
	payload := map[string]string{
		"variety": variety,
		"orderid": req.OrderID,
	}
	body, _ := json.Marshal(payload)
	url := b.baseURL() + angelPathCancelOrder
	httpReq, err := httpNewJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setAngelHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "cancel_order", httpReq)
	if err != nil {
		return brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	return decodeAngelJSON(resp, nil)
}

func (b *AngelBroker) GetAccountEquity(ctx context.Context) (float64, error) {
	url := b.baseURL() + angelPathGetRMS
	httpReq, err := httpNewJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	b.setAngelHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "get_equity", httpReq)
	if err != nil {
		return 0, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}

	var result struct {
		Data struct {
			AvailableCash string `json:"availablecash"`
		} `json:"data"`
	}
	if err := decodeAngelJSON(resp, &result); err != nil {
		return 0, err
	}
	equity, err := strconv.ParseFloat(strings.TrimSpace(result.Data.AvailableCash), 64)
	if err != nil || equity <= 0 {
		return 0, brokererr.New(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal))
	}
	return equity, nil
}

func (b *AngelBroker) resolveInstrument(ctx context.Context, exchange, symbol string) (token, tradingsymbol string, err error) {
	if b.infra == nil || b.infra.SymbolTokens == nil {
		return b.searchScrip(ctx, exchange, symbol)
	}
	token, tradingsymbol, err = b.infra.SymbolTokens.Resolve(ctx, "angel", exchange, symbol, b.searchScrip)
	if err != nil {
		if err == symboltoken.ErrNotFound {
			return "", "", brokererr.New(brokererr.CodeInvalidSymbol, brokererr.PublicMessage(brokererr.CodeInvalidSymbol))
		}
		return "", "", err
	}
	return token, tradingsymbol, nil
}

type angelSearchRow struct {
	Exchange      string `json:"exchange"`
	TradingSymbol string `json:"tradingsymbol"`
	SymbolToken   string `json:"symboltoken"`
}

func (b *AngelBroker) searchScrip(ctx context.Context, exchange, symbol string) (token, tradingsymbol string, err error) {
	payload := map[string]string{
		"exchange":    strings.ToUpper(strings.TrimSpace(exchange)),
		"searchscrip": strings.TrimSpace(symbol),
	}
	body, _ := json.Marshal(payload)
	url := b.baseURL() + angelPathSearchScrip
	httpReq, err := httpNewJSONRequest(ctx, http.MethodPost, url, body)
	if err != nil {
		return "", "", err
	}
	b.setAngelHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "search_scrip", httpReq)
	if err != nil {
		return "", "", brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}

	var result struct {
		Data []angelSearchRow `json:"data"`
	}
	if err := decodeAngelJSON(resp, &result); err != nil {
		return "", "", err
	}
	return pickAngelSearchResult(symbol, result.Data)
}

func pickAngelSearchResult(symbol string, rows []angelSearchRow) (token, tradingsymbol string, err error) {
	if len(rows) == 0 {
		return "", "", symboltoken.ErrNotFound
	}
	symUpper := strings.ToUpper(strings.TrimSpace(symbol))
	for _, r := range rows {
		if strings.EqualFold(r.TradingSymbol, symbol) {
			return r.SymbolToken, r.TradingSymbol, nil
		}
	}
	eqCandidate := symUpper + "-EQ"
	for _, r := range rows {
		if strings.EqualFold(r.TradingSymbol, eqCandidate) {
			return r.SymbolToken, r.TradingSymbol, nil
		}
	}
	return rows[0].SymbolToken, rows[0].TradingSymbol, nil
}

func (b *AngelBroker) setAngelHeaders(req *http.Request) {
	h := b.infra.Angel
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+b.parsed.JWT)
	req.Header.Set("X-PrivateKey", b.parsed.APIKey)
	req.Header.Set("X-UserType", "USER")
	req.Header.Set("X-SourceID", "WEB")
	req.Header.Set("X-ClientLocalIP", h.ClientLocalIP)
	req.Header.Set("X-ClientPublicIP", h.ClientPublicIP)
	req.Header.Set("X-MACAddress", h.MACAddress)
}

func angelProductType(product string) string {
	switch strings.ToUpper(strings.TrimSpace(product)) {
	case "MIS":
		return "INTRADAY"
	case "CNC":
		return "DELIVERY"
	case "NRML":
		return "CARRYFORWARD"
	default:
		return "INTRADAY"
	}
}

type angelEnvelope struct {
	Status    bool   `json:"status"`
	Message   string `json:"message"`
	ErrorCode string `json:"errorcode"`
}

func decodeAngelJSON(resp *http.Response, dest any) error {
	body, err := readResponseBody(resp)
	if err != nil {
		return brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return brokerErrFromStatus(resp)
	}

	var env angelEnvelope
	if err := json.Unmarshal(body, &env); err == nil {
		if !env.Status {
			msg := strings.TrimSpace(env.Message)
			if msg == "" {
				msg = brokererr.PublicMessage(mapHTTPErrorCode(resp.StatusCode, msg))
			}
			code := mapHTTPErrorCode(resp.StatusCode, msg)
			if mentionsNoPosition(msg) {
				code = brokererr.CodeNoPosition
			}
			return brokererr.New(code, msg)
		}
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
