package livebrokers

import (
	"github.com/SPSingh09/zettabridge/internal/domain"
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
)

// ZerodhaBroker is a Kite Connect adapter (live HTTP, market orders only in v1).
type ZerodhaBroker struct {
	adapterBase
	parsed brokercreds.Parsed
}

func (b *ZerodhaBroker) PlaceOrder(ctx context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	if req.Action == "CLOSE" {
		return b.squareOff(ctx, req)
	}
	if req.OrderType == domain.OrderBracket {
		return b.placeCoverOrder(ctx, req)
	}
	return b.placeMarketOrder(ctx, indianTxnType(req.Action), req)
}

func (b *ZerodhaBroker) placeMarketOrder(ctx context.Context, txnType string, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	form := url.Values{
		"tradingsymbol":    {req.Symbol},
		"exchange":         {req.Exchange},
		"transaction_type": {txnType},
		"product":          {req.Product},
		"quantity":         {strconv.Itoa(int(req.Quantity))},
	}

	if strings.ToUpper(req.EntryExecType) == "LIMIT" {
		price := req.Price
		if price <= 0 {
			ltp, err := b.getLTP(ctx, req.Exchange, req.Symbol)
			if err != nil {
				return nil, wrapLTPLookupErr(err, "could not fetch live market price to auto-price this LIMIT order (add an explicit price to the signal to skip this lookup)")
			}
			price = b.limitPrice(ltp, txnType, req.MarketProtection)
		}
		form.Set("order_type", "LIMIT")
		form.Set("price", strconv.FormatFloat(price, 'f', 2, 64))
	} else {
		form.Set("order_type", "MARKET")
		if req.MarketProtection > 0 {
			form.Set("market_protection", strconv.FormatFloat(req.MarketProtection, 'f', 2, 64))
		}
	}

	if tag := strings.TrimSpace(req.AlgoID); tag != "" {
		form.Set("tag", tag)
	}

	endpoint := b.baseURL() + "/orders/regular"
	httpReq, err := httpNewFormRequest(ctx, http.MethodPost, endpoint, form)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setKiteHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "place_order", httpReq)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	if !isSuccessStatus(resp.StatusCode) {
		return nil, brokerErrFromStatus(resp)
	}

	var result struct {
		Data struct {
			OrderID string `json:"order_id"`
		} `json:"data"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	out := submittedResult("ZERODHA")
	out.OrderID = result.Data.OrderID
	return out, nil
}

// limitPrice computes the LIMIT order price at LTP ± slippage%, direction-aware.
func (b *ZerodhaBroker) limitPrice(ltp float64, txnType string, slippagePct float64) float64 {
	if slippagePct <= 0 {
		return math.Round(ltp*100) / 100
	}
	factor := slippagePct / 100
	if strings.ToUpper(txnType) == "BUY" {
		return math.Round(ltp*(1+factor)*100) / 100
	}
	return math.Round(ltp*(1-factor)*100) / 100
}

func (b *ZerodhaBroker) getLTP(ctx context.Context, exchange, symbol string) (float64, error) {
	key := fmt.Sprintf("%s:%s", strings.ToUpper(exchange), strings.ToUpper(symbol))
	url := b.baseURL() + "/quote?i=" + key
	httpReq, err := httpNewJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setKiteHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "get_ltp", httpReq)
	if err != nil {
		return 0, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	if !isSuccessStatus(resp.StatusCode) {
		return 0, brokerErrFromStatus(resp)
	}

	var result struct {
		Data map[string]struct {
			LastPrice float64 `json:"last_price"`
		} `json:"data"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		return 0, err
	}
	entry, ok := result.Data[key]
	if !ok || entry.LastPrice <= 0 {
		return 0, brokererr.New(brokererr.CodeInvalidSymbol, brokererr.PublicMessage(brokererr.CodeInvalidSymbol))
	}
	return entry.LastPrice, nil
}

// wrapLTPLookupErr adds context to a failed live-price lookup. The raw
// broker error alone (e.g. "[permission_denied] Insufficient permission for
// that call.") doesn't explain that ZettaBridge needed the market-data/quote
// API to auto-price this order — without that, a quote-API permission-scope
// error reads just like an order-placement failure with no clue why a quote
// was even requested.
func wrapLTPLookupErr(err error, context string) error {
	code, msg := brokererr.UserFacing(err)
	return brokererr.New(code, context+": "+msg)
}

func (b *ZerodhaBroker) placeCoverOrder(ctx context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	if req.TPPoints > 0 {
		log.Printf("zerodha: cover order does not support TP; tp_pts=%d ignored symbol=%s", req.TPPoints, req.Symbol)
	}

	ltp, err := b.getLTP(ctx, req.Exchange, req.Symbol)
	if err != nil {
		return nil, wrapLTPLookupErr(err, "could not fetch live market price needed to compute the stop-loss trigger price")
	}

	txnType := indianTxnType(req.Action)
	var slPrice float64
	if strings.ToUpper(req.Action) == "SELL" {
		slPrice = ltp + float64(req.SLPoints)
	} else {
		slPrice = ltp - float64(req.SLPoints)
	}
	if slPrice <= 0 {
		slPrice = 0.05
	}

	form := url.Values{
		"tradingsymbol":    {req.Symbol},
		"exchange":         {req.Exchange},
		"transaction_type": {txnType},
		"product":          {req.Product},
		"quantity":         {strconv.Itoa(int(req.Quantity))},
		"trigger_price":    {strconv.FormatFloat(slPrice, 'f', 2, 64)},
	}
	if strings.ToUpper(req.EntryExecType) == "LIMIT" {
		entryPrice := req.Price
		if entryPrice <= 0 {
			entryPrice = b.limitPrice(ltp, txnType, req.MarketProtection)
		}
		form.Set("order_type", "LIMIT")
		form.Set("price", strconv.FormatFloat(entryPrice, 'f', 2, 64))
	} else {
		form.Set("order_type", "MARKET")
		if req.MarketProtection > 0 {
			form.Set("market_protection", strconv.FormatFloat(req.MarketProtection, 'f', 2, 64))
		}
	}
	if tag := strings.TrimSpace(req.AlgoID); tag != "" {
		form.Set("tag", tag)
	}

	endpoint := b.baseURL() + "/orders/co"
	httpReq, err := httpNewFormRequest(ctx, http.MethodPost, endpoint, form)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setKiteHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "place_co", httpReq)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	if !isSuccessStatus(resp.StatusCode) {
		return nil, brokerErrFromStatus(resp)
	}

	var result struct {
		Data struct {
			OrderID string `json:"order_id"`
		} `json:"data"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		return nil, err
	}
	out := submittedResult("ZERODHA-CO")
	out.OrderID = result.Data.OrderID
	out.SLPrice = slPrice
	return out, nil
}

func (b *ZerodhaBroker) squareOff(ctx context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	netQty, err := b.netPositionQty(ctx, req.Symbol, req.Exchange, req.Product)
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
	closeReq.Quantity = float64(qty)
	closeReq.EntryExecType = "MARKET" // CLOSE must fill immediately; never send a LIMIT close order
	return b.placeMarketOrder(ctx, txnType, &closeReq)
}

type kiteNetPosition struct {
	TradingSymbol string  `json:"tradingsymbol"`
	Exchange      string  `json:"exchange"`
	Product       string  `json:"product"`
	Quantity      float64 `json:"quantity"`
}

func (b *ZerodhaBroker) netPositionQty(ctx context.Context, symbol, exchange, product string) (int, error) {
	url := b.baseURL() + "/portfolio/positions"
	httpReq, err := httpNewJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	b.setKiteHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "get_positions", httpReq)
	if err != nil {
		return 0, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, brokerErrFromStatus(resp)
	}

	var result struct {
		Data struct {
			Net []kiteNetPosition `json:"net"`
		} `json:"data"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		return 0, err
	}

	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	exchange = strings.ToUpper(strings.TrimSpace(exchange))
	product = strings.ToUpper(strings.TrimSpace(product))

	for _, p := range result.Data.Net {
		if strings.ToUpper(p.TradingSymbol) != symbol {
			continue
		}
		if strings.ToUpper(p.Exchange) != exchange {
			continue
		}
		if strings.ToUpper(p.Product) != product {
			continue
		}
		return int(math.Round(p.Quantity)), nil
	}
	return 0, nil
}

func (b *ZerodhaBroker) CancelOrder(ctx context.Context, req *domain.CancelRequest) error {
	variety := req.Variety
	if variety == "" {
		variety = "regular"
	}
	url := b.baseURL() + "/orders/" + variety + "/" + req.OrderID
	httpReq, err := httpNewJSONRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}
	b.setKiteHeaders(httpReq)

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

func (b *ZerodhaBroker) GetAccountEquity(ctx context.Context) (float64, error) {
	url := b.baseURL() + "/user/margins"
	httpReq, err := httpNewJSONRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	b.setKiteHeaders(httpReq)

	resp, err := b.infra.HTTP.Do(ctx, b.cred.BrokerType, "get_equity", httpReq)
	if err != nil {
		return 0, brokererr.Wrap(brokererr.CodeBrokerUnreachable, brokererr.PublicMessage(brokererr.CodeBrokerUnreachable), err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, brokerErrFromStatus(resp)
	}

	var result struct {
		Data struct {
			Equity struct {
				Available struct {
					Cash float64 `json:"cash"`
				} `json:"available"`
			} `json:"equity"`
		} `json:"data"`
	}
	if err := decodeJSON(resp, &result); err != nil {
		return 0, err
	}
	return result.Data.Equity.Available.Cash, nil
}

func (b *ZerodhaBroker) setKiteHeaders(req *http.Request) {
	req.Header.Set("Authorization", "token "+b.parsed.APIKey+":"+b.parsed.AccessToken)
	req.Header.Set("X-Kite-Version", "3")
}

func indianTxnType(action string) string {
	if strings.ToUpper(action) == "SELL" {
		return "SELL"
	}
	return "BUY"
}
