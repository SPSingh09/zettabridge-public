package pnl

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/SPSingh09/zettabridge/internal/transport/http/middleware"
	"github.com/SPSingh09/zettabridge/internal/modules/credentials"
	"github.com/SPSingh09/zettabridge/internal/modules/shared"
	"github.com/SPSingh09/zettabridge/internal/plan"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// Handler handles P&L and trade export endpoints.
type Handler struct {
	*shared.Handler
}

func (h *Handler) GetWebhookPnL(c *fiber.Ctx) error {
	webhookID := c.Params("id")
	wh, err := h.RequireWebhookAccess(c, webhookID, false)
	if err != nil {
		return err
	}

	summary, err := h.PG.GetWebhookPnL(c.Context(), webhookID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load pnl")
	}

	if wh.BrokerCredID != nil {
		var cred *store.BrokerCredential
		if c, err := h.PG.GetBrokerCred(c.Context(), *wh.BrokerCredID); err == nil && c != nil {
			cred = c
			summary.BrokerType = cred.BrokerType
			summary.ExecutionMode = cred.ExecutionMode
			if cred.BrokerType == "zerodha" && cred.ExecutionMode != "publisher" {
				summary.ZerodhaConnectionStatus = credentials.ZerodhaTokenStatus(cred.ConnectedAt)
			}
		}
		// Zerodha OAuth fill status is synced by the background fill-poll job.
		// Other live brokers still only record broker accept (submitted), so a
		// computed FillRate would read as 0% and mislead — hide it for them.
		if cred == nil || cred.BrokerType != "zerodha" || cred.ExecutionMode == "publisher" {
			summary.FillRate = nil
		}
	} else if wh.PaperAccountID != nil {
		if account, err := h.PG.GetPaperAccount(c.Context(), *wh.PaperAccountID); err == nil && account != nil {
			summary.PaperAccountLabel = account.Label
			if profile, err := h.PG.GetMarketProfileByCode(c.Context(), account.MarketProfile); err == nil && profile != nil {
				summary.MarketProfileName = profile.Name
			}
		}
	}

	return shared.Ok(c, summary)
}

func (h *Handler) GetOrgPnL(c *fiber.Ctx) error {
	orgID := c.Params("id")
	summary, err := h.PG.GetOrgPnL(c.Context(), orgID)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load pnl")
	}
	return shared.Ok(c, summary)
}

func (h *Handler) ExportTrades(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	userPlan, err := h.UserPlan(c, uid)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not load user plan")
	}

	webhookID := c.Params("id")
	wh, err := h.RequireWebhookAccess(c, webhookID, false)
	if err != nil {
		return err
	}

	// Paper Trading is a flat, plan-independent tier — exempt from the
	// Individual+ audit-log/export gate the same way EnforceAdvancedGuardsPlan
	// exempts paper webhooks from guard-config plan gating.
	if wh.PaperAccountID == nil {
		if err := plan.CanViewAuditLogs(userPlan); err != nil {
			return shared.PlanError(c, err)
		}
	}

	from, to, err := ParseDateRange(c)
	if err != nil {
		return shared.Fail(c, fiber.StatusBadRequest, err.Error())
	}

	trades, err := h.PG.ListTradesForExport(c.Context(), webhookID, from, to)
	if err != nil {
		return shared.Fail(c, fiber.StatusInternalServerError, "could not fetch trades")
	}

	format := c.Query("format", "csv")
	if format == "json" {
		if trades == nil {
			trades = []*store.Trade{}
		}
		return shared.Ok(c, trades)
	}

	c.Set("Content-Type", "text/csv")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="trades-%s.csv"`, webhookID))

	w := csv.NewWriter(c.Response().BodyWriter())
	_ = w.Write([]string{
		"request_id", "webhook_id", "webhook_label",
		"signal", "symbol", "lot_size", "order_type", "product",
		"status", "fill_price", "sl_price", "tp_price",
		"error", "error_code", "created_at",
	})
	for _, t := range trades {
		sl := ""
		if t.SLPrice != nil {
			sl = strconv.FormatFloat(*t.SLPrice, 'f', -1, 64)
		}
		tp := ""
		if t.TPPrice != nil {
			tp = strconv.FormatFloat(*t.TPPrice, 'f', -1, 64)
		}
		_ = w.Write([]string{
			t.ID,
			t.WebhookID,
			t.WebhookLabel,
			t.Signal,
			t.Symbol,
			strconv.FormatFloat(t.LotSize, 'f', -1, 64),
			t.OrderType,
			t.Product,
			t.Status,
			strconv.FormatFloat(t.FillPrice, 'f', -1, 64),
			sl,
			tp,
			t.Error,
			t.ErrorCode,
			t.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	w.Flush()
	return nil
}

// ParseDateRange parses optional "from" and "to" query params (YYYY-MM-DD).
func ParseDateRange(c *fiber.Ctx) (from, to *time.Time, err error) {
	if v := c.Query("from"); v != "" {
		t, e := time.Parse("2006-01-02", v)
		if e != nil {
			return nil, nil, fmt.Errorf("invalid from date (expected YYYY-MM-DD)")
		}
		from = &t
	}
	if v := c.Query("to"); v != "" {
		t, e := time.Parse("2006-01-02", v)
		if e != nil {
			return nil, nil, fmt.Errorf("invalid to date (expected YYYY-MM-DD)")
		}
		// to is exclusive upper bound: advance to start of next day
		next := t.AddDate(0, 0, 1)
		to = &next
	}
	return from, to, nil
}
