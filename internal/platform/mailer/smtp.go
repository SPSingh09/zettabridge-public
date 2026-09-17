package mailer

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"
)

// SMTPSender sends mail via SMTP (Mailhog, SendGrid relay, Zoho, etc.).
type SMTPSender struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

func (s *SMTPSender) SendZerodhaReconnect(_ context.Context, to, reconnectURL string) error {
	return s.send(to, "Action required: reconnect your Zerodha account", zerodhaReconnectBody(reconnectURL))
}

func (s *SMTPSender) SendFyersReconnect(_ context.Context, to, reconnectURL, reason string) error {
	return s.send(to, "Action required: reconnect FYERS market data", fyersReconnectBody(reconnectURL, reason))
}

func (s *SMTPSender) SendVerification(_ context.Context, to, verifyURL string) error {
	return s.send(to, "Verify your ZettaBridge email", verificationBody(verifyURL))
}

func (s *SMTPSender) SendOrgInvite(_ context.Context, to, orgName, inviteURL string) error {
	return s.send(to, "You've been invited to join "+orgName+" on ZettaBridge", orgInviteBody(orgName, inviteURL))
}

func (s *SMTPSender) SendPlatformInvite(_ context.Context, to, inviteURL string) error {
	return s.send(to, "Your ZettaBridge invitation", platformInviteBody(inviteURL))
}

func (s *SMTPSender) SendSymbolRequestReceived(_ context.Context, to, symbol, exchange, marketProfile, reason, requesterEmail, adminURL string) error {
	return s.send(to, "New symbol request: "+symbol, symbolRequestReceivedBody(symbol, exchange, marketProfile, reason, requesterEmail, adminURL))
}

func (s *SMTPSender) SendSymbolRequestStatusUpdate(_ context.Context, to, symbol, status, adminNote, requestsURL string) error {
	return s.send(to, "Symbol request update: "+symbol, symbolRequestStatusUpdateBody(symbol, status, adminNote, requestsURL))
}

func (s *SMTPSender) SendPaperTradeQuotaExceeded(_ context.Context, to, upgradeURL string, limit int) error {
	return s.send(to, "Paper trade limit reached on ZettaBridge", paperTradeQuotaBody(upgradeURL, limit))
}

func (s *SMTPSender) send(to, subject, body string) error {
	if s.Host == "" {
		return fmt.Errorf("smtp: host not configured")
	}
	port := s.Port
	if port <= 0 {
		port = 587
	}
	from := strings.TrimSpace(s.From)
	if from == "" {
		from = "noreply@localhost"
	}
	msg := strings.Join([]string{
		"From: " + from,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		body,
	}, "\r\n")
	addr := fmt.Sprintf("%s:%d", s.Host, port)
	var auth smtp.Auth
	if s.Username != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}
	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}
