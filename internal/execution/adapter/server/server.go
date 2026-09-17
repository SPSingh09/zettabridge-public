package server

import (
	"context"
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"github.com/SPSingh09/zettabridge/internal/execution"
	"github.com/SPSingh09/zettabridge/internal/execution/adapter/local"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
)

// Executor runs execution commands for an adapter binary.
type Executor interface {
	PlaceOrder(ctx context.Context, cmd execution.ExecutionCommand) (execution.ExecutionOutcome, error)
	CancelOrder(ctx context.Context, cmd execution.CancelCommand) (execution.ExecutionOutcome, error)
}

// LiveExecutor wraps the in-process live adapter.
type LiveExecutor struct {
	Live *local.LiveAdapter
}

func (e *LiveExecutor) PlaceOrder(ctx context.Context, cmd execution.ExecutionCommand) (execution.ExecutionOutcome, error) {
	req := cmd.ToExecutionRequest()
	result, err := e.Live.Execute(ctx, req)
	if err != nil {
		code, msg := brokererr.TradeFields(err)
		brokerType := ""
		var ee *local.ExecutionError
		if errors.As(err, &ee) {
			brokerType = ee.BrokerType
			code, msg = brokererr.TradeFields(ee.Err)
		}
		return execution.ExecutionOutcome{
			RequestID:    cmd.RequestID,
			Status:       "rejected",
			BrokerType:   brokerType,
			ErrorCode:    code,
			ErrorMessage: msg,
		}, nil
	}
	return execution.OutcomeFromResult(cmd.RequestID, result, result.BrokerType), nil
}

func (e *LiveExecutor) CancelOrder(_ context.Context, _ execution.CancelCommand) (execution.ExecutionOutcome, error) {
	return execution.ExecutionOutcome{}, fiber.NewError(fiber.StatusNotImplemented, "cancel not implemented on adapter stub")
}

// PaperExecutor wraps the paper engine via domain.ExecutionDestination.
type PaperExecutor struct {
	Paper *local.PaperAdapter
}

func (e *PaperExecutor) PlaceOrder(ctx context.Context, cmd execution.ExecutionCommand) (execution.ExecutionOutcome, error) {
	req := cmd.ToExecutionRequest()
	result, err := e.Paper.Execute(ctx, req)
	if err != nil {
		return execution.ExecutionOutcome{
			RequestID:    cmd.RequestID,
			Status:       "rejected",
			ErrorMessage: err.Error(),
		}, nil
	}
	return execution.OutcomeFromResult(cmd.RequestID, result, "paper"), nil
}

func (e *PaperExecutor) CancelOrder(_ context.Context, _ execution.CancelCommand) (execution.ExecutionOutcome, error) {
	return execution.ExecutionOutcome{}, fiber.NewError(fiber.StatusNotImplemented, "cancel not implemented on paper adapter stub")
}

// Config configures an adapter HTTP server.
type Config struct {
	Name         string
	Port         string
	Executor     Executor
	ServiceToken string
	Idempotency  IdempotencyStore
	// MountPublic registers browser-facing callback routes (OAuth, publisher).
	// Called before any service-token-protected routes so Kite redirects stay public.
	MountPublic func(app *fiber.App)
}

// NewApp builds a Fiber app with adapter routes.
func NewApp(cfg Config) *fiber.App {
	app := fiber.New(fiber.Config{AppName: "ZettaBridge " + cfg.Name + " Adapter"})
	app.Use(recover.New())

	if cfg.MountPublic != nil {
		cfg.MountPublic(app)
	}

	exec := cfg.Executor
	if cfg.Idempotency != nil && cfg.Executor != nil {
		exec = &IdempotentExecutor{Inner: cfg.Executor, Store: cfg.Idempotency}
	}

	app.Get("/v1/healthz", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "adapter": cfg.Name})
	})
	app.Get("/v1/capabilities", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"adapter": cfg.Name, "orders": true, "cancel": false})
	})

	var orderAuth fiber.Handler
	if cfg.ServiceToken != "" {
		orderAuth = middleware.RequireServiceToken(cfg.ServiceToken)
	}

	app.Post("/v1/orders", orderAuth, func(c *fiber.Ctx) error {
		var cmd execution.ExecutionCommand
		if err := c.BodyParser(&cmd); err != nil {
			return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
		}
		out, err := exec.PlaceOrder(c.Context(), cmd)
		if err != nil {
			var fe *fiber.Error
			if errors.As(err, &fe) {
				return shared.Fail(c, fe.Code, fe.Message)
			}
			return shared.Fail(c, fiber.StatusInternalServerError, err.Error())
		}
		return shared.Ok(c, out)
	})

	app.Post("/v1/orders/cancel", orderAuth, func(c *fiber.Ctx) error {
		var cmd execution.CancelCommand
		if err := c.BodyParser(&cmd); err != nil {
			return shared.Fail(c, fiber.StatusBadRequest, "invalid body")
		}
		out, err := exec.CancelOrder(c.Context(), cmd)
		if err != nil {
			var fe *fiber.Error
			if errors.As(err, &fe) {
				return shared.Fail(c, fe.Code, fe.Message)
			}
			return shared.Fail(c, fiber.StatusInternalServerError, err.Error())
		}
		return shared.Ok(c, out)
	})

	return app
}

// Run starts the adapter HTTP server.
func Run(cfg Config) error {
	app := NewApp(cfg)
	addr := ":" + cfg.Port
	log.Printf("%s adapter listening on %s", cfg.Name, addr)
	return app.Listen(addr)
}
