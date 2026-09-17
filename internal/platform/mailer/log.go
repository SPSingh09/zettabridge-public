package mailer

import (
	"context"
	"log"
)

// LogSender prints verification links to the server log (local dev / CI).
type LogSender struct{}

func (LogSender) SendVerification(_ context.Context, to, verifyURL string) error {
	log.Printf("email_verify: to=%s url=%s", to, verifyURL)
	return nil
}

func (LogSender) SendZerodhaReconnect(_ context.Context, to, reconnectURL string) error {
	log.Printf("zerodha_reconnect: to=%s url=%s", to, reconnectURL)
	return nil
}

func (LogSender) SendFyersReconnect(_ context.Context, to, reconnectURL, reason string) error {
	log.Printf("fyers_reconnect: to=%s url=%s reason=%q", to, reconnectURL, reason)
	return nil
}

func (LogSender) SendOrgInvite(_ context.Context, to, orgName, inviteURL string) error {
	log.Printf("org_invite: to=%s org=%q url=%s", to, orgName, inviteURL)
	return nil
}

func (LogSender) SendPlatformInvite(_ context.Context, to, inviteURL string) error {
	log.Printf("platform_invite: to=%s url=%s", to, inviteURL)
	return nil
}

func (LogSender) SendSymbolRequestReceived(_ context.Context, to, symbol, exchange, marketProfile, reason, requesterEmail, adminURL string) error {
	log.Printf("symbol_request_received: to=%s symbol=%s exchange=%s profile=%s requester=%s reason=%q url=%s",
		to, symbol, exchange, marketProfile, requesterEmail, reason, adminURL)
	return nil
}

func (LogSender) SendSymbolRequestStatusUpdate(_ context.Context, to, symbol, status, adminNote, requestsURL string) error {
	log.Printf("symbol_request_status_update: to=%s symbol=%s status=%s note=%q url=%s", to, symbol, status, adminNote, requestsURL)
	return nil
}

func (LogSender) SendPaperTradeQuotaExceeded(_ context.Context, to, upgradeURL string, limit int) error {
	log.Printf("paper_trade_quota: to=%s limit=%d url=%s", to, limit, upgradeURL)
	return nil
}
