// Package fyers is the FYERS market data provider for paper trading's
// mark-to-market background job (never used for order placement — LTP
// quotes only).
//
// The /data/quotes endpoint, request/response shape (Authorization header as
// "appID:accessToken", response envelope {"s":"ok","d":[{"n":...,"v":{"lp":...}}]})
// are taken from FYERS's official fyers-apiv3 Python SDK (Config.quotes,
// Config.DATA_API) plus FYERS's own support-portal quotes API example —
// not guessed from unofficial sources.
package fyers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/SPSingh09/zettabridge/internal/marketdata"
)

// quotesURL is a var (not const) so tests can point it at an httptest server.
var quotesURL = "https://api-t1.fyers.in/data/quotes"

// Provider is the FYERS-backed marketdata.Provider implementation. It holds
// the current access token in memory — kept fresh by
// internal/integrations/brokers/fyers/refresh.Job calling SetAccessToken
// after a successful login or silent refresh — rather than hitting the
// database on every quote fetch.
type Provider struct {
	AppID string

	mu          sync.RWMutex
	accessToken string
}

// New returns a Provider with no access token yet — GetLTP/GetBatchLTP
// report marketdata.ErrNoQuote until SetAccessToken is called.
func New(appID string) *Provider {
	return &Provider{AppID: appID}
}

// SetAccessToken updates the in-memory token used for API calls.
func (p *Provider) SetAccessToken(token string) {
	p.mu.Lock()
	p.accessToken = token
	p.mu.Unlock()
}

func (p *Provider) currentToken() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.accessToken
}

// GetLTP fetches a single quote via GetBatchLTP.
func (p *Provider) GetLTP(ctx context.Context, symbol marketdata.Symbol) (*marketdata.Quote, error) {
	quotes, err := p.GetBatchLTP(ctx, []marketdata.Symbol{symbol})
	if err != nil {
		return nil, err
	}
	if len(quotes) == 0 {
		return nil, marketdata.ErrNoQuote
	}
	return &quotes[0], nil
}

// GetBatchLTP fetches quotes for up to 50 symbols in one call (FYERS's
// documented limit — not enforced here, callers should chunk if needed).
// Symbols are mapped to FYERS's "EXCHANGE:SYMBOL-EQ" format — equity
// cash-market only, matching the current Indian Equity Paper Trading MVP
// scope (marketdata.Symbol has no segment/series field yet for F&O/indices).
func (p *Provider) GetBatchLTP(ctx context.Context, symbols []marketdata.Symbol) ([]marketdata.Quote, error) {
	token := p.currentToken()
	if token == "" || p.AppID == "" || len(symbols) == 0 {
		return nil, marketdata.ErrNoQuote
	}

	fyersSymbols := make([]string, len(symbols))
	bySymbol := make(map[string]marketdata.Symbol, len(symbols))
	for i, s := range symbols {
		fs := s.Exchange + ":" + s.Symbol + "-EQ"
		fyersSymbols[i] = fs
		bySymbol[fs] = s
	}

	reqURL := quotesURL + "?symbols=" + strings.Join(fyersSymbols, ",")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fyers: build request: %w", err)
	}
	req.Header.Set("Authorization", p.AppID+":"+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("version", "3")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fyers: unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fyers: quotes %d: %s", resp.StatusCode, body)
	}

	var result struct {
		S string `json:"s"`
		D []struct {
			N string `json:"n"`
			V struct {
				LP float64 `json:"lp"`
			} `json:"v"`
		} `json:"d"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("fyers: parse error: %w", err)
	}
	if result.S != "ok" {
		return nil, fmt.Errorf("fyers: quotes request failed: %s", body)
	}

	quotes := make([]marketdata.Quote, 0, len(result.D))
	for _, d := range result.D {
		sym, ok := bySymbol[d.N]
		if !ok || d.V.LP <= 0 {
			continue
		}
		quotes = append(quotes, marketdata.Quote{Symbol: sym, LTP: d.V.LP})
	}
	if len(quotes) == 0 {
		return nil, marketdata.ErrNoQuote
	}
	return quotes, nil
}
