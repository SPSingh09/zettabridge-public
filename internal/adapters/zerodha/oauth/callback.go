package oauth

import (
	"context"
	"log"
	"net/url"

	"github.com/gofiber/fiber/v2"

	"github.com/SPSingh09/zettabridge/internal/brokercreds"
	"github.com/SPSingh09/zettabridge/internal/config"
	zerodhaauth "github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/auth"
	zerodhasettings "github.com/SPSingh09/zettabridge/internal/integrations/brokers/zerodha/settings"
	credenc "github.com/SPSingh09/zettabridge/internal/platform/crypto"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// Store loads and updates broker credentials during OAuth.
type Store interface {
	GetBrokerCred(ctx context.Context, id string) (*store.BrokerCredential, error)
	ReconnectZerodhaToken(ctx context.Context, credID, encryptedCreds string) error
	GetPlatformSetting(ctx context.Context, key string) (value string, ok bool, err error)
}

// SessionNotifier notifies Core when running on the adapter (optional).
type SessionNotifier interface {
	NotifySessionUpdate(credID, encryptedCreds string) error
}

// Deps holds OAuth callback dependencies.
type Deps struct {
	Cfg      *config.Config
	Store    Store
	Notifier SessionNotifier // nil when Core updates PG directly
}

// HandleCallback processes Kite OAuth browser redirect (public, no JWT).
func HandleCallback(c *fiber.Ctx, deps Deps) error {
	cfg := deps.Cfg
	frontendURL := cfg.EffectiveDashboardURL()

	failRedirect := func(reason string) error {
		log.Printf("zerodha_oauth: callback failed reason=%s status=%q request_token=%v state=%v",
			reason, c.Query("status"), c.Query("request_token") != "", c.Query("state") != "")
		return c.Redirect(frontendURL + "/credentials?zerodha=error&reason=" + url.QueryEscape(reason))
	}

	if !zerodhasettings.OAuthEnabled(c.Context(), deps.Store) {
		return failRedirect("zerodha oauth disabled")
	}
	if c.Query("status") != "success" {
		return failRedirect("cancelled")
	}
	requestToken := c.Query("request_token")
	state := c.Query("state")
	if requestToken == "" || state == "" {
		return failRedirect("missing params")
	}

	userID, credID, _, err := zerodhaauth.ParseState(state, cfg.JWTSecret)
	if err != nil {
		log.Printf("zerodha_oauth: invalid state: %v", err)
		return failRedirect("invalid state")
	}
	if credID == "" {
		return failRedirect("missing credential id")
	}

	cred, err := deps.Store.GetBrokerCred(c.Context(), credID)
	if err != nil || cred == nil {
		log.Printf("zerodha_oauth: callback cred lookup cred=%s user=%s: %v", credID, userID, err)
		return failRedirect("credential not found")
	}

	credKey, err := cfg.AESKeyBytes()
	if err != nil {
		return failRedirect("server error")
	}
	plaintext, err := credenc.PlaintextOrDecrypt(cred.EncryptedCreds, credKey, cred.ID)
	if err != nil {
		log.Printf("zerodha_oauth: callback decrypt cred=%s: %v", credID, err)
		return failRedirect("server error")
	}
	parsed, err := brokercreds.Parse("zerodha", plaintext)
	if err != nil {
		log.Printf("zerodha_oauth: callback parse cred=%s: %v", credID, err)
		return failRedirect("credential format invalid")
	}

	accessToken, err := zerodhaauth.ExchangeToken(parsed.APIKey, parsed.APISecret, requestToken)
	if err != nil {
		log.Printf("zerodha_oauth: token exchange failed for user %s: %v", userID, err)
		return failRedirect("token exchange failed")
	}

	rawCreds := parsed.APIKey + ":" + parsed.APISecret + ":" + accessToken
	encrypted, err := credenc.Encrypt(rawCreds, credKey, credID)
	if err != nil {
		return failRedirect("server error")
	}

	if deps.Notifier != nil {
		if err := deps.Notifier.NotifySessionUpdate(credID, encrypted); err != nil {
			log.Printf("zerodha_oauth: core session notify cred=%s: %v", credID, err)
			return failRedirect("server error")
		}
	} else if err := deps.Store.ReconnectZerodhaToken(c.Context(), credID, encrypted); err != nil {
		log.Printf("zerodha_oauth: update cred=%s user=%s failed: %v", credID, userID, err)
		return failRedirect("server error")
	}

	log.Printf("zerodha_oauth: connected cred=%s user=%s", credID, userID)
	return c.Redirect(frontendURL + "/credentials?zerodha=connected")
}
