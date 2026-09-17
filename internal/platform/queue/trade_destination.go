package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/credentialproduct"
	"github.com/SPSingh09/zettabridge/internal/guard"
	"github.com/SPSingh09/zettabridge/internal/integrations/brokers/brokererr"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// resolveWebhookProduct picks the broker/paper product for this signal before the
// trade audit row is inserted. Paper webhooks use the paper account default;
// live webhooks resolve against the credential's enabled product list (required
// when multiple products such as MIS+CNC are enabled).
func (q *Queue) resolveWebhookProduct(ctx context.Context, wh *store.Webhook, params *guard.TradeParams) error {
	if q == nil || wh == nil || params == nil {
		return nil
	}
	params.Product = strings.ToUpper(strings.TrimSpace(params.Product))

	if wh.PaperAccountID != nil && *wh.PaperAccountID != "" {
		account, err := q.pg.GetPaperAccount(ctx, *wh.PaperAccountID)
		if err != nil {
			return fmt.Errorf("paper account lookup failed")
		}
		if account == nil {
			return fmt.Errorf("paper account not found")
		}
		if params.Product == "" {
			params.Product = strings.ToUpper(strings.TrimSpace(account.DefaultProduct))
		}
		return nil
	}

	if wh.BrokerCredID != nil && *wh.BrokerCredID != "" {
		cred, err := q.pg.GetBrokerCred(ctx, *wh.BrokerCredID)
		if err != nil {
			return fmt.Errorf("broker credentials not found")
		}
		if cred == nil {
			return fmt.Errorf("broker credentials not found")
		}
		product, err := credentialproduct.ResolveOrderProduct(cred.Product, params.Product)
		if err != nil {
			return err
		}
		params.Product = product
	}
	return nil
}

func tradeRejectCodeForProductErr(err error) string {
	if err == nil {
		return string(brokererr.CodeInternal)
	}
	msg := err.Error()
	if strings.Contains(msg, "product is required") || strings.Contains(msg, "not enabled on this broker account") {
		return "product_required"
	}
	if strings.Contains(msg, "paper account") {
		return "paper_account_not_found"
	}
	if errors.Is(err, guard.ErrOrderTypeNotAllowed) {
		return guard.ErrCode(err)
	}
	return string(brokererr.CodeInternal)
}
