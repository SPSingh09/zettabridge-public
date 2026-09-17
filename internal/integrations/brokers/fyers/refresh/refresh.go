// Package fyersrefresh runs a periodic background job that keeps the
// platform's single FYERS market-data connection alive: it silently
// refreshes the access token before it expires using FYERS's undocumented
// refresh-token+PIN flow (see fyersauth package docs — best-effort, not
// officially supported), and notifies the admin who connected it if that
// fails, so they can reconnect manually from the admin dashboard.
package fyersrefresh

import (
	"context"
	"fmt"
	"log"
	"time"

	fyersauth "github.com/SPSingh09/zettabridge/internal/integrations/brokers/fyers/auth"
	"github.com/SPSingh09/zettabridge/internal/integrations/telegram"
	credenc "github.com/SPSingh09/zettabridge/internal/platform/crypto"
	"github.com/SPSingh09/zettabridge/internal/platform/mailer"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// fyersConnectionAAD is the fixed additional-authenticated-data tag used to
// encrypt/decrypt the single-row fyers_connection secrets.
const fyersConnectionAAD = "fyers_connection"

// refreshBefore is how far ahead of expiry the job attempts a refresh.
const refreshBefore = 2 * time.Hour

// TokenSetter is the subset of marketdata/fyers.Provider the job needs —
// depend on the interface, not the concrete type.
type TokenSetter interface {
	SetAccessToken(token string)
}

// Store is the narrow persistence surface the job needs — mirrors the
// paperengine.Engine / marketdata.SnapshotJob precedent of depending on an
// interface, not the concrete *store.PGStore, so this is unit-testable.
type Store interface {
	GetFyersConnection(ctx context.Context) (*store.FyersConnection, error)
	UpsertFyersTokens(ctx context.Context, accessToken, refreshToken string, accessExpiresAt, refreshExpiresAt time.Time, connectedBy string) error
	GetUserByID(ctx context.Context, id string) (*store.User, error)
}

// Job periodically loads the FYERS connection, refreshes it if close to
// expiry, and keeps Provider's in-memory token current.
type Job struct {
	store        Store
	provider     TokenSetter
	appID        string
	secretID     string
	credKey      []byte
	mail         mailer.Sender
	tg           *telegram.Sender
	interval     time.Duration
	reconnectURL string
}

// New returns a Job. Call Start to begin the background ticker.
func New(s Store, provider TokenSetter, appID, secretID string, credKey []byte, mail mailer.Sender, tg *telegram.Sender, interval time.Duration, reconnectURL string) *Job {
	return &Job{
		store: s, provider: provider, appID: appID, secretID: secretID, credKey: credKey,
		mail: mail, tg: tg, interval: interval, reconnectURL: reconnectURL,
	}
}

// Start runs the tick loop until ctx is cancelled. Non-blocking.
func (j *Job) Start(ctx context.Context) {
	go func() {
		// Load whatever token exists immediately so a server restart doesn't
		// leave Provider empty until the first ticker fire.
		j.tick(ctx)

		ticker := time.NewTicker(j.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				j.tick(ctx)
			}
		}
	}()
}

func (j *Job) tick(ctx context.Context) {
	conn, err := j.store.GetFyersConnection(ctx)
	if err != nil {
		log.Printf("fyersrefresh: load connection failed: %v", err)
		return
	}
	if conn == nil || conn.EncryptedAccessToken == "" {
		return // never connected — nothing to do
	}

	if token, err := credenc.Decrypt(conn.EncryptedAccessToken, j.credKey, fyersConnectionAAD); err == nil && token != "" {
		j.provider.SetAccessToken(token)
	}

	if conn.AccessTokenExpiresAt == nil || time.Until(*conn.AccessTokenExpiresAt) > refreshBefore {
		return // not close to expiry yet
	}
	if conn.EncryptedRefreshToken == "" || conn.EncryptedPIN == "" {
		j.notifyReconnect(ctx, conn, "no refresh token or PIN on file")
		return
	}

	refreshToken, err := credenc.Decrypt(conn.EncryptedRefreshToken, j.credKey, fyersConnectionAAD)
	if err != nil {
		log.Printf("fyersrefresh: decrypt refresh token failed: %v", err)
		return
	}
	pin, err := credenc.Decrypt(conn.EncryptedPIN, j.credKey, fyersConnectionAAD)
	if err != nil {
		log.Printf("fyersrefresh: decrypt pin failed: %v", err)
		return
	}

	result, err := fyersauth.RefreshAccessToken(j.appID, j.secretID, refreshToken, pin)
	if err != nil {
		log.Printf("fyersrefresh: silent refresh failed: %v", err)
		j.notifyReconnect(ctx, conn, err.Error())
		return
	}

	newAccessEncrypted, err := credenc.Encrypt(result.AccessToken, j.credKey, fyersConnectionAAD)
	if err != nil {
		log.Printf("fyersrefresh: encrypt new access token failed: %v", err)
		return
	}
	newRefreshToken := result.RefreshToken
	if newRefreshToken == "" {
		newRefreshToken = refreshToken // FYERS may not rotate it on every refresh
	}
	newRefreshEncrypted, err := credenc.Encrypt(newRefreshToken, j.credKey, fyersConnectionAAD)
	if err != nil {
		log.Printf("fyersrefresh: encrypt new refresh token failed: %v", err)
		return
	}

	accessExpiresAt := time.Now().Add(fyersauth.AccessTokenValidity)
	refreshExpiresAt := time.Now().Add(fyersauth.RefreshTokenValidity)
	if err := j.store.UpsertFyersTokens(ctx, newAccessEncrypted, newRefreshEncrypted, accessExpiresAt, refreshExpiresAt, ""); err != nil {
		log.Printf("fyersrefresh: store refreshed tokens failed: %v", err)
		return
	}
	j.provider.SetAccessToken(result.AccessToken)
	log.Printf("fyersrefresh: silently refreshed access token, valid until %s", accessExpiresAt.Format(time.RFC3339))
}

func (j *Job) notifyReconnect(ctx context.Context, conn *store.FyersConnection, reason string) {
	if conn.ConnectedBy == nil {
		return
	}
	user, err := j.store.GetUserByID(ctx, *conn.ConnectedBy)
	if err != nil || user == nil {
		log.Printf("fyersrefresh: could not look up connecting admin to notify: %v", err)
		return
	}
	log.Printf("fyersrefresh: notifying %s to reconnect FYERS (reason: %s)", user.Email, reason)
	if j.mail != nil {
		if err := j.mail.SendFyersReconnect(ctx, user.Email, j.reconnectURL, reason); err != nil {
			log.Printf("fyersrefresh: email to %s failed: %v", user.Email, err)
		}
	}
	if j.tg != nil && user.TelegramChatID != "" {
		msg := fmt.Sprintf(
			"⚠️ <b>Reconnect FYERS</b>\n\nSilent token refresh failed (%s). Paper Trading market data will pause until reconnected.\n\n<a href=\"%s\">Reconnect now</a>",
			reason, j.reconnectURL,
		)
		if err := j.tg.Send(ctx, user.TelegramChatID, msg); err != nil {
			log.Printf("fyersrefresh: telegram to chat %s failed: %v", user.TelegramChatID, err)
		}
	}
}
