package local

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/SPSingh09/zettabridge/internal/algo"
	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/domain"
	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokerfactory"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/livebrokers"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/maporder"
	zerodhasettings "github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/settings"
	"github.com/SPSingh09/zettabridge/internal/plan"
	credenc "github.com/SPSingh09/zettabridge/internal/platform/crypto"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// ExecutionError wraps a live-execution failure with broker type for metrics tagging.
type ExecutionError struct {
	Err        error
	BrokerType string
}

func (e *ExecutionError) Error() string { return e.Err.Error() }
func (e *ExecutionError) Unwrap() error { return e.Err }

type BrokerFactory func(mode string, infra *livebrokers.Infra, cred *store.BrokerCredential, parsed brokercreds.Parsed) (domain.Broker, error)

type RateLimiter interface {
	IncrBrokerCredRateLimit(ctx context.Context, brokerCredID string) (int64, error)
}

type BrokerGate interface {
	RequireBroker(brokerType string) error
}

type LiveStore interface {
	GetBrokerCred(ctx context.Context, id string) (*store.BrokerCredential, error)
	GetUserByID(ctx context.Context, id string) (*store.User, error)
	GetPlatformSetting(ctx context.Context, key string) (value string, ok bool, err error)
}

type PublisherLinker interface {
	LinkPublisherOrderTrade(ctx context.Context, publisherOrderID, tradeID string) error
}

// LiveAdapter executes live broker orders in-process (phase 1).
type LiveAdapter struct {
	pg         LiveStore
	redis      RateLimiter
	credKey    []byte
	brokerMode string
	infra      *livebrokers.Infra
	newBroker  BrokerFactory
	algoPolicy algo.Policy
	router     BrokerGate
	linker     PublisherLinker
}

type LiveDeps struct {
	Store      LiveStore
	Redis      RateLimiter
	CredKey    []byte
	BrokerMode string
	Infra      *livebrokers.Infra
	NewBroker  BrokerFactory
	AlgoPolicy algo.Policy
	Router     BrokerGate
	Linker     PublisherLinker
}

func NewLive(deps LiveDeps) *LiveAdapter {
	newBroker := deps.NewBroker
	if newBroker == nil {
		newBroker = brokerfactory.New
	}
	return &LiveAdapter{
		pg:         deps.Store,
		redis:      deps.Redis,
		credKey:    deps.CredKey,
		brokerMode: deps.BrokerMode,
		infra:      deps.Infra,
		newBroker:  newBroker,
		algoPolicy: deps.AlgoPolicy,
		router:     deps.Router,
		linker:     deps.Linker,
	}
}

func (l *LiveAdapter) SetBrokerFactory(f BrokerFactory) {
	l.newBroker = f
}

func (l *LiveAdapter) Router() BrokerGate {
	return l.router
}

func (l *LiveAdapter) Execute(ctx context.Context, req domain.ExecutionRequest) (*domain.ExecutionResult, error) {
	cred, err := l.pg.GetBrokerCred(ctx, req.AccountID)
	if err != nil || cred == nil {
		return nil, &ExecutionError{Err: brokererr.New(brokererr.CodeInvalidCredentials, "broker credentials not found"), BrokerType: ""}
	}
	brokerType := cred.BrokerType

	if l.router != nil {
		if err := l.router.RequireBroker(brokerType); err != nil {
			return nil, &ExecutionError{Err: brokererr.New(brokererr.CodeInternal, err.Error()), BrokerType: brokerType}
		}
	}

	credLimit := int64(plan.BrokerCredOrdersPerSecCap)
	credCount, err := l.redis.IncrBrokerCredRateLimit(ctx, cred.ID)
	if err != nil {
		log.Printf("worker: broker cred rate-limit redis error: %v", err)
	} else if credCount > credLimit {
		return nil, &ExecutionError{
			Err:        brokererr.New(brokererr.CodeRateLimited, fmt.Sprintf("broker credential rate limit exceeded (%d req/s)", credLimit)),
			BrokerType: brokerType,
		}
	}

	user, err := l.pg.GetUserByID(ctx, req.UserID)
	if err != nil || user == nil {
		return nil, &ExecutionError{Err: brokererr.New(brokererr.CodeInternal, "user not found"), BrokerType: brokerType}
	}
	userPlan := plan.EffectivePlan(user.Plan)

	route, err := brokerfactory.ResolveExecution(l.brokerMode, userPlan, cred)
	if err != nil {
		return nil, &ExecutionError{Err: brokererr.New(tradeCodeForLiveAccess(err), brokererr.PublicMessage(tradeCodeForLiveAccess(err))), BrokerType: brokerType}
	}

	plaintext, err := credenc.PlaintextOrDecrypt(cred.EncryptedCreds, l.credKey, cred.ID)
	if err != nil {
		return nil, &ExecutionError{Err: brokererr.New(brokererr.CodeInvalidCredentials, "credential decrypt failed"), BrokerType: brokerType}
	}
	p, err := brokercreds.Parse(cred.BrokerType, plaintext)
	if err != nil {
		return nil, &ExecutionError{Err: brokererr.New(brokererr.CodeInvalidCredentials, brokererr.PublicMessage(brokererr.CodeInvalidCredentials)), BrokerType: brokerType}
	}
	parsed := p
	cred.EncryptedCreds = plaintext

	execCred := route.CredentialForExecution(cred)
	if execCred.BrokerType == "zerodha" {
		isPublisher := domain.ExecutionMode(execCred.ExecutionMode) == domain.ExecutionModePublisher
		var zerodhaOK bool
		if isPublisher {
			zerodhaOK = zerodhasettings.PublisherEnabled(ctx, l.pg)
		} else {
			zerodhaOK = zerodhasettings.OAuthEnabled(ctx, l.pg)
		}
		if !zerodhaOK {
			msg := "zerodha User OAuth API execution is currently disabled by the platform admin"
			if isPublisher {
				msg = "zerodha Kite Publisher execution is currently disabled by the platform admin"
			}
			return nil, &ExecutionError{Err: brokererr.New(brokererr.CodeInternal, msg), BrokerType: brokerType}
		}
	}
	b, err := l.newBroker(route.Mode, l.infra, execCred, parsed)
	if err != nil {
		return nil, &ExecutionError{Err: brokererr.New(brokererr.CodeInvalidCredentials, err.Error()), BrokerType: brokerType}
	}

	lot, err := ResolveLot(ctx, b, cred.BrokerType, req.Quantity, req.FixedLotSize, req.MaxRiskPct, req.SLPoints)
	if err != nil {
		return nil, &ExecutionError{Err: brokererr.New(brokererr.CodeInternal, err.Error()), BrokerType: brokerType}
	}
	lot, err = guard.ApplyMaxLotSize(req.MaxLotSize, lot)
	if err != nil {
		return nil, &ExecutionError{Err: brokererr.New(brokererr.CodeInternal, err.Error()), BrokerType: brokerType}
	}

	params := guard.TradeParams{Symbol: req.Symbol, Price: req.Price, SLPts: req.SLPoints, TPPts: req.TPPoints, Product: req.Product}
	placeReq, err := maporder.Build(req.OrderType, cred, params, lot)
	if err != nil {
		return nil, &ExecutionError{Err: err, BrokerType: brokerType}
	}
	maporder.WithAction(placeReq, req.Side)
	placeReq.Comment = req.Comment

	algoID, err := algo.ResolveForExecution(cred, route, l.algoPolicy)
	if err != nil {
		return nil, &ExecutionError{Err: err, BrokerType: brokerType}
	}
	placeReq.AlgoID = algoID

	orderResult, err := b.PlaceOrder(ctx, placeReq)
	if err != nil {
		return nil, &ExecutionError{Err: err, BrokerType: brokerType}
	}

	result := &domain.ExecutionResult{
		OrderID:    orderResult.OrderID,
		Quantity:   lot,
		OrderType:  placeReq.OrderType,
		AlgoID:     algoID,
		BrokerType: brokerType,
	}
	switch orderResult.Status {
	case domain.StatusFilled:
		result.Status = domain.StatusFilled
	case domain.StatusRejected:
		result.Status = domain.StatusRejected
	case domain.StatusPendingConfirmation:
		result.Status = domain.StatusPendingConfirmation
	default:
		result.Status = domain.StatusSubmitted
	}
	result.FillPrice = orderResult.FillPrice
	result.SLPrice = orderResult.SLPrice
	result.TPPrice = orderResult.TPPrice
	result.Handoff = orderResult.Handoff

	if result.Status == domain.StatusPendingConfirmation && orderResult.OrderID != "" && l.linker != nil {
		if err := l.linker.LinkPublisherOrderTrade(ctx, orderResult.OrderID, req.SignalID); err != nil {
			log.Printf("worker: link publisher order trade failed publisher_order=%s trade=%s: %v",
				orderResult.OrderID, req.SignalID, err)
		}
	}

	return result, nil
}

func tradeCodeForLiveAccess(err error) brokererr.Code {
	switch {
	case errors.Is(err, plan.ErrLiveNotAllowed):
		return brokererr.CodeLiveNotAllowed
	default:
		return brokererr.CodeInternal
	}
}
