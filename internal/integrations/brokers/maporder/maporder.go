package maporder

import (
	"fmt"
	"log"

	"github.com/SPSingh09/zettabridge/internal/credentialproduct"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"math"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/store"
)

var indianBrokers = map[string]struct{}{
	"zerodha": {}, "angel": {}, "dhan": {},
}

// Common forex symbols that are invalid on Indian equity brokers.
var forexSymbols = map[string]struct{}{
	"EURUSD": {}, "GBPUSD": {}, "USDJPY": {}, "USDCHF": {}, "AUDUSD": {},
	"USDCAD": {}, "NZDUSD": {}, "EURGBP": {}, "EURJPY": {}, "GBPJPY": {},
	"XAUUSD": {}, "XAGUSD": {},
}

// Build converts a webhook order type + signal params into a broker PlaceRequest.
// defaultOrderType is the webhook's DefaultOrderType (ExecutionRequest.OrderType
// on the live path). Empty values default to LIMIT.
func Build(defaultOrderType string, cred *store.BrokerCredential, params guard.TradeParams, lot float64) (*domain.PlaceRequest, error) {
	if cred == nil {
		return nil, brokererr.New(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal))
	}

	symbol := strings.TrimSpace(params.Symbol)
	if symbol == "" {
		return nil, brokererr.New(brokererr.CodeInvalidSymbol, brokererr.PublicMessage(brokererr.CodeInvalidSymbol))
	}
	if err := validateSymbolForBroker(cred.BrokerType, symbol); err != nil {
		return nil, err
	}

	req := &domain.PlaceRequest{
		Action:     "",
		Symbol:     symbol,
		SLPoints:   params.SLPts,
		TPPoints:   params.TPPts,
		Price:      params.Price,
		BrokerType: cred.BrokerType,
		OrderType:  domain.OrderMarket,
	}
	if params.SLPts > 0 || params.TPPts > 0 {
		req.OrderType = domain.OrderBracket
	}

	if cred.BrokerType == "mt5_cloud" {
		req.Exchange = ""
		req.Product = ""
		req.Quantity = lot
		req.QuantityUnit = domain.QuantityLots
		return req, nil
	}

	if _, ok := indianBrokers[cred.BrokerType]; ok {
		ex := strings.TrimSpace(cred.Exchange)
		if ex == "" {
			return nil, brokererr.New(brokererr.CodeInvalidCredentials, "exchange must be configured on credential (NSE or BSE)")
		}
		ex = strings.ToUpper(ex)
		if ex != "NSE" && ex != "BSE" {
			return nil, brokererr.New(brokererr.CodeInvalidCredentials, "exchange must be NSE or BSE")
		}
		req.Exchange = ex
		product, err := credentialproduct.ResolveOrderProduct(cred.Product, params.Product)
		if err != nil {
			return nil, brokererr.New(brokererr.CodeInvalidCredentials, err.Error())
		}
		// Zerodha CO (/orders/co) only supports MIS; CNC and NRML are rejected by the API.
		if req.OrderType == domain.OrderBracket && product != "MIS" {
			return nil, brokererr.New(brokererr.CodeBracketNotSupported,
				brokererr.PublicMessage(brokererr.CodeBracketNotSupported))
		}
		req.Product = product
		// Indian broker quantity is in shares directly — no lot-to-share conversion.
		shares := int(math.Round(lot))
		if shares < 1 {
			shares = 1
		}
		req.Quantity = float64(shares)
		req.QuantityUnit = domain.QuantityShares

		execType := strings.ToUpper(strings.TrimSpace(defaultOrderType))
		if execType == "" {
			execType = "LIMIT"
		}
		if execType != "MARKET" && execType != "LIMIT" {
			return nil, brokererr.New(brokererr.CodeInternal, "order type must be MARKET or LIMIT")
		}
		req.EntryExecType = execType
		req.MarketProtection = cred.MarketProtection
		log.Printf("maporder: DIAG cred=%s defaultOrderType_in=%q execType_out=%q", cred.ID, defaultOrderType, execType)
		return req, nil
	}

	return nil, brokererr.New(brokererr.CodeInvalidCredentials, fmt.Sprintf("unsupported broker_type %q", cred.BrokerType))
}

func validateSymbolForBroker(brokerType, symbol string) error {
	if brokerType == "mt5_cloud" {
		return nil
	}
	if _, ok := indianBrokers[brokerType]; !ok {
		return nil
	}
	if _, ok := forexSymbols[strings.ToUpper(symbol)]; ok {
		return brokererr.New(brokererr.CodeInvalidSymbol, brokererr.PublicMessage(brokererr.CodeInvalidSymbol))
	}
	return nil
}

// WithAction sets the signal action on a built request.
func WithAction(req *domain.PlaceRequest, action string) *domain.PlaceRequest {
	if req != nil {
		req.Action = action
	}
	return req
}
