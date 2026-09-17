package publisher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// KiteFormFields is the complete POST form payload for kite.zerodha.com/connect/basket.
// Stored in publisher_orders.basket_payload_json so the GET endpoint can reconstruct
// the handoff without credential decryption.
type KiteFormFields struct {
	APIKey string `json:"api_key"`
	Data   string `json:"data"`
}

// PublisherStore is the narrow store interface the executor needs.
type PublisherStore interface {
	InsertPublisherOrder(ctx context.Context, o *store.PublisherOrder) error
	InsertPublisherOrderEvent(ctx context.Context, e *store.PublisherOrderEvent) error
}

// Executor implements domain.Broker for Zerodha Kite Publisher mode.
// PlaceOrder builds a basket handoff instead of calling the Kite API directly.
// CancelOrder and GetAccountEquity are unsupported in this mode.
type Executor struct {
	cred        *store.BrokerCredential
	parsed      brokercreds.Parsed
	pg          PublisherStore
	jwtSecret   string
	callbackURL string // e.g. https://api.zettabridge.net/v1/publisher/callback
}

// NewExecutor constructs a publisher executor.
func NewExecutor(
	cred *store.BrokerCredential,
	parsed brokercreds.Parsed,
	pg PublisherStore,
	jwtSecret string,
	callbackURL string,
) *Executor {
	return &Executor{
		cred:        cred,
		parsed:      parsed,
		pg:          pg,
		jwtSecret:   jwtSecret,
		callbackURL: callbackURL,
	}
}

// PlaceOrder builds a Kite basket handoff and stores the publisher order.
// Returns OrderResult with Status=pending_confirmation and Handoff fields for the dashboard.
func (e *Executor) PlaceOrder(ctx context.Context, req *domain.PlaceRequest) (*domain.OrderResult, error) {
	if err := validate(e.parsed, req); err != nil {
		return nil, brokererr.New(brokererr.CodeUnsupportedAction, err.Error())
	}

	item, err := mapOrder(req)
	if err != nil {
		return nil, brokererr.New(brokererr.CodeUnsupportedAction, err.Error())
	}

	basketJSON, err := buildBasketJSON([]BasketItem{item})
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}

	ff := KiteFormFields{APIKey: e.parsed.APIKey, Data: basketJSON}
	formJSON, err := json.Marshal(ff)
	if err != nil {
		return nil, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}

	orderID := uuid.New().String()
	signedState := GenerateState(orderID, e.jwtSecret)
	now := time.Now().UTC()

	po := &store.PublisherOrder{
		ID:                orderID,
		UserID:            e.cred.UserID,
		CredentialID:      e.cred.ID,
		Broker:            "zerodha",
		ExecutionMode:     "publisher",
		BasketPayloadJSON: formJSON,
		BasketPayloadHash: sha256Hex(string(formJSON)),
		Status:            "created",
		SignedState:       signedState,
		ExpiresAt:         now.Add(stateMaxAge),
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if e.cred.OrgID != nil && *e.cred.OrgID != "" {
		// webhook_id set by caller via LinkPublisherOrderTrade after trade row created
	}

	if err := e.pg.InsertPublisherOrder(ctx, po); err != nil {
		return nil, brokererr.Wrap(brokererr.CodeInternal, brokererr.PublicMessage(brokererr.CodeInternal), err)
	}

	if err := e.pg.InsertPublisherOrderEvent(ctx, &store.PublisherOrderEvent{
		ID:               uuid.New().String(),
		PublisherOrderID: orderID,
		EventType:        "BASKET_CREATED",
		CreatedAt:        now,
	}); err != nil {
		// non-fatal: audit event failure should not block the handoff
		_ = err
	}

	return &domain.OrderResult{
		Status:  domain.StatusPendingConfirmation,
		OrderID: orderID,
		Handoff: &domain.HandoffResult{
			Provider:         "zerodha",
			Method:           "POST_FORM",
			Action:           kiteBasketURL,
			Fields:           map[string]string{"api_key": e.parsed.APIKey, "data": basketJSON},
			PublisherOrderID: orderID,
		},
	}, nil
}

// CancelOrder is not supported in publisher mode.
func (e *Executor) CancelOrder(_ context.Context, _ *domain.CancelRequest) error {
	return brokererr.New(brokererr.CodeUnsupportedAction,
		"Zerodha Publisher mode does not support cancel — cancel the order directly on Kite")
}

// GetAccountEquity is not supported in publisher mode.
func (e *Executor) GetAccountEquity(_ context.Context) (float64, error) {
	return 0, brokererr.New(brokererr.CodeUnsupportedAction,
		"Zerodha Publisher mode does not support account equity fetch")
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
