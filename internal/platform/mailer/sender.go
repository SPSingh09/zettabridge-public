package mailer

import (
	"context"
	"fmt"
	"strings"

	"github.com/SPSingh09/zettabridge/internal/config"
)

// Sender delivers transactional email.
type Sender interface {
	SendVerification(ctx context.Context, to, verifyURL string) error
	SendZerodhaReconnect(ctx context.Context, to, reconnectURL string) error
	SendFyersReconnect(ctx context.Context, to, reconnectURL, reason string) error
	SendOrgInvite(ctx context.Context, to, orgName, inviteURL string) error
	SendPlatformInvite(ctx context.Context, to, inviteURL string) error
	SendSymbolRequestReceived(ctx context.Context, to, symbol, exchange, marketProfile, reason, requesterEmail, adminURL string) error
	SendSymbolRequestStatusUpdate(ctx context.Context, to, symbol, status, adminNote, requestsURL string) error
	SendPaperTradeQuotaExceeded(ctx context.Context, to, upgradeURL string, limit int) error
}

// New builds the configured email sender (log or smtp).
func New(cfg *config.Config) Sender {
	switch strings.ToLower(strings.TrimSpace(cfg.EmailProvider)) {
	case "smtp":
		return &SMTPSender{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUser,
			Password: cfg.SMTPPass,
			From:     cfg.EmailFrom,
		}
	default:
		return LogSender{}
	}
}

func orgInviteBody(orgName, inviteURL string) string {
	return emailLayout(
		"You've been invited",
		fmt.Sprintf("You have been invited to join the <strong>%s</strong> workspace on ZettaBridge.", orgName),
		"Accept Invitation",
		inviteURL,
		"This link expires in 7 days. If you did not expect this invitation, you can safely ignore this email.",
	)
}

func platformInviteBody(inviteURL string) string {
	return emailLayout(
		"You're invited to ZettaBridge",
		"You have been invited to create a ZettaBridge account. Click below to complete your registration.",
		"Create Account",
		inviteURL,
		"This link expires in 7 days. If you did not expect this invitation, you can safely ignore this email.",
	)
}

func zerodhaReconnectBody(reconnectURL string) string {
	return emailLayout(
		"Reconnect your Zerodha account",
		"Your Zerodha connection expires at <strong>6:00 AM IST</strong> today. Reconnect before then to keep your live trading active.",
		"Reconnect Zerodha",
		reconnectURL,
		"If you have already reconnected, you can ignore this email.",
	)
}

func fyersReconnectBody(reconnectURL, reason string) string {
	return emailLayout(
		"Reconnect FYERS market data",
		fmt.Sprintf("ZettaBridge's silent FYERS token refresh failed (%s). Paper Trading price updates from FYERS will pause until you reconnect.", reason),
		"Reconnect FYERS",
		reconnectURL,
		"This only affects the Paper Trading market data feed, not live broker trading.",
	)
}

func symbolRequestReceivedBody(symbol, exchange, marketProfile, reason, requesterEmail, adminURL string) string {
	return emailLayout(
		"New symbol request",
		fmt.Sprintf(
			"<strong>%s</strong> requested the symbol <strong>%s</strong> (%s / %s) be added to the instrument master.<br><br>Reason: %s",
			requesterEmail, symbol, exchange, marketProfile, reason,
		),
		"Review in Admin",
		adminURL,
		"You're receiving this because you're configured as ZettaBridge's support/admin notification contact.",
	)
}

func symbolRequestStatusUpdateBody(symbol, status, adminNote, requestsURL string) string {
	heading := map[string]string{
		"accepted": "Your symbol request was accepted",
		"rejected": "Your symbol request was rejected",
		"resolved": "Your symbol has been added",
	}[status]
	if heading == "" {
		heading = "Your symbol request was updated"
	}
	body := fmt.Sprintf("Your request to add <strong>%s</strong> is now <strong>%s</strong>.", symbol, status)
	if adminNote != "" {
		body += fmt.Sprintf("<br><br>Note from the ZettaBridge team: %s", adminNote)
	}
	return emailLayout(heading, body, "View Your Requests", requestsURL,
		"You're receiving this because you submitted a symbol request on ZettaBridge.")
}

func verificationBody(verifyURL string) string {
	return emailLayout(
		"Verify your email address",
		"Thanks for signing up for ZettaBridge. Please verify your email address to activate your account.",
		"Verify Email",
		verifyURL,
		"This link expires in 24 hours. If you did not register, you can safely ignore this email.",
	)
}

func paperTradeQuotaBody(upgradeURL string, limit int) string {
	return emailLayout(
		"Paper trade limit reached",
		fmt.Sprintf(
			"You've used all <strong>%d</strong> paper trades included on your Free plan this month. Upgrade for unlimited paper trading, or wait until next month when your quota resets.",
			limit,
		),
		"Upgrade plan",
		upgradeURL,
		"You're receiving this because a paper trade was rejected after your monthly quota was used.",
	)
}

// emailLayout renders a minimal, inbox-safe HTML email with a single CTA button.
// All styles are inline so they survive Gmail, Outlook, and Apple Mail stripping.
func emailLayout(heading, bodyText, ctaLabel, ctaURL, footer string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head><meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:#f4f4f5;font-family:Arial,sans-serif">
  <table width="100%%" cellpadding="0" cellspacing="0" style="background:#f4f4f5;padding:40px 0">
    <tr><td align="center">
      <table width="560" cellpadding="0" cellspacing="0" style="background:#ffffff;border-radius:8px;overflow:hidden;max-width:560px;width:100%%">

        <!-- Header -->
        <tr>
          <td style="background:#0f172a;padding:24px 32px">
            <span style="color:#ffffff;font-size:18px;font-weight:700;letter-spacing:-0.3px">ZettaBridge</span>
          </td>
        </tr>

        <!-- Body -->
        <tr>
          <td style="padding:32px 32px 24px">
            <h1 style="margin:0 0 12px;font-size:22px;font-weight:700;color:#0f172a">%s</h1>
            <p style="margin:0 0 28px;font-size:15px;line-height:1.6;color:#374151">%s</p>
            <a href="%s"
               style="display:inline-block;padding:13px 28px;background:#2563eb;color:#ffffff;font-size:15px;font-weight:600;text-decoration:none;border-radius:6px">
              %s
            </a>
            <p style="margin:20px 0 0;font-size:13px;color:#6b7280">
              Or copy this link into your browser:<br>
              <a href="%s" style="color:#2563eb;word-break:break-all">%s</a>
            </p>
          </td>
        </tr>

        <!-- Footer -->
        <tr>
          <td style="padding:20px 32px 28px;border-top:1px solid #e5e7eb">
            <p style="margin:0;font-size:12px;color:#9ca3af">%s</p>
          </td>
        </tr>

      </table>
    </td></tr>
  </table>
</body>
</html>`, heading, bodyText, ctaURL, ctaLabel, ctaURL, ctaURL, footer)
}
