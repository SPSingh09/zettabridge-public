package queue

import (
	"context"

	"github.com/SPSingh09/zettabridge/internal/store"
)

func brokerTypeForWebhook(ctx context.Context, pg TradeStore, wh *store.Webhook) string {
	if wh == nil || pg == nil {
		return ""
	}
	if wh.PaperAccountID != nil {
		return "paper"
	}
	if wh.BrokerCredID == nil {
		return ""
	}
	cred, err := pg.GetBrokerCred(ctx, *wh.BrokerCredID)
	if err != nil || cred == nil {
		return ""
	}
	return cred.BrokerType
}
