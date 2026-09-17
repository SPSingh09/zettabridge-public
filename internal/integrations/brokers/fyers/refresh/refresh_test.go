package fyersrefresh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	fyersauth "github.com/SPSingh09/zettabridge/internal/integrations/brokers/fyers/auth"
	credenc "github.com/SPSingh09/zettabridge/internal/platform/crypto"
	"github.com/SPSingh09/zettabridge/internal/store"
)

// testCredKey is a fixed 32-byte AES key — never used outside this test file.
var testCredKey = []byte("0123456789012345678901234567890a")

func encryptForTest(t *testing.T, plaintext string) string {
	t.Helper()
	blob, err := credenc.Encrypt(plaintext, testCredKey, fyersConnectionAAD)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return blob
}

// fakeStore is an in-memory Store for testing the refresh job without a database.
type fakeStore struct {
	conn        *store.FyersConnection
	users       map[string]*store.User
	upsertCalls int
	lastAccess  string
	lastRefresh string
}

func (f *fakeStore) GetFyersConnection(_ context.Context) (*store.FyersConnection, error) {
	return f.conn, nil
}

func (f *fakeStore) UpsertFyersTokens(_ context.Context, accessToken, refreshToken string, accessExpiresAt, refreshExpiresAt time.Time, connectedBy string) error {
	f.upsertCalls++
	f.lastAccess = accessToken
	f.lastRefresh = refreshToken
	f.conn.EncryptedAccessToken = accessToken
	f.conn.EncryptedRefreshToken = refreshToken
	f.conn.AccessTokenExpiresAt = &accessExpiresAt
	f.conn.RefreshTokenExpiresAt = &refreshExpiresAt
	return nil
}

func (f *fakeStore) GetUserByID(_ context.Context, id string) (*store.User, error) {
	return f.users[id], nil
}

// fakeProvider records SetAccessToken calls.
type fakeProvider struct {
	tokens []string
}

func (p *fakeProvider) SetAccessToken(token string) {
	p.tokens = append(p.tokens, token)
}

// fakeMailer records SendFyersReconnect calls; other methods are unused stubs.
type fakeMailer struct {
	reconnectCalls int
	lastReason     string
}

func (f *fakeMailer) SendVerification(_ context.Context, _, _ string) error { return nil }
func (f *fakeMailer) SendZerodhaReconnect(_ context.Context, _, _ string) error { return nil }
func (f *fakeMailer) SendFyersReconnect(_ context.Context, _, _, reason string) error {
	f.reconnectCalls++
	f.lastReason = reason
	return nil
}
func (f *fakeMailer) SendOrgInvite(_ context.Context, _, _, _ string) error      { return nil }
func (f *fakeMailer) SendPlatformInvite(_ context.Context, _, _ string) error    { return nil }
func (f *fakeMailer) SendSymbolRequestReceived(_ context.Context, _, _, _, _, _, _, _ string) error {
	return nil
}
func (f *fakeMailer) SendSymbolRequestStatusUpdate(_ context.Context, _, _, _, _, _ string) error {
	return nil
}
func (f *fakeMailer) SendPaperTradeQuotaExceeded(_ context.Context, _, _ string, _ int) error {
	return nil
}

func TestTick_NoConnection_NoOp(t *testing.T) {
	fs := &fakeStore{conn: nil, users: map[string]*store.User{}}
	fp := &fakeProvider{}
	mail := &fakeMailer{}
	job := New(fs, fp, "APP", "SECRET", testCredKey, mail, nil, time.Minute, "https://example.com/admin/settings")

	job.tick(context.Background())

	if len(fp.tokens) != 0 {
		t.Fatalf("expected no SetAccessToken calls, got %d", len(fp.tokens))
	}
	if mail.reconnectCalls != 0 {
		t.Fatalf("expected no reconnect emails, got %d", mail.reconnectCalls)
	}
}

func TestTick_ValidTokenNotNearExpiry_LoadsIntoProviderNoRefresh(t *testing.T) {
	encryptedAccess := encryptForTest(t, "current-access-token")
	farFuture := time.Now().Add(20 * time.Hour)
	fs := &fakeStore{
		conn: &store.FyersConnection{
			EncryptedAccessToken: encryptedAccess,
			AccessTokenExpiresAt: &farFuture,
		},
		users: map[string]*store.User{},
	}
	fp := &fakeProvider{}
	mail := &fakeMailer{}
	job := New(fs, fp, "APP", "SECRET", testCredKey, mail, nil, time.Minute, "https://example.com/admin/settings")

	job.tick(context.Background())

	if len(fp.tokens) != 1 || fp.tokens[0] != "current-access-token" {
		t.Fatalf("expected provider to be loaded with current token, got %v", fp.tokens)
	}
	if fs.upsertCalls != 0 {
		t.Fatalf("expected no refresh attempt while token isn't near expiry, got %d upsert calls", fs.upsertCalls)
	}
}

func TestTick_NearExpiryMissingRefreshData_NotifiesReconnect(t *testing.T) {
	encryptedAccess := encryptForTest(t, "current-access-token")
	soon := time.Now().Add(1 * time.Hour) // within refreshBefore window
	fs := &fakeStore{
		conn: &store.FyersConnection{
			EncryptedAccessToken: encryptedAccess,
			AccessTokenExpiresAt: &soon,
			ConnectedBy:          strPtr("admin-1"),
			// No refresh token or PIN on file.
		},
		users: map[string]*store.User{"admin-1": {ID: "admin-1", Email: "admin@example.com"}},
	}
	fp := &fakeProvider{}
	mail := &fakeMailer{}
	job := New(fs, fp, "APP", "SECRET", testCredKey, mail, nil, time.Minute, "https://example.com/admin/settings")

	job.tick(context.Background())

	if mail.reconnectCalls != 1 {
		t.Fatalf("expected exactly one reconnect email, got %d", mail.reconnectCalls)
	}
}

func TestTick_NearExpiryWithRefreshData_SilentlyRefreshes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "refresh_token" || body["refresh_token"] != "old-refresh-token" || body["pin"] != "1234" {
			t.Errorf("unexpected refresh request body: %+v", body)
		}
		w.Write([]byte(`{"s":"ok","access_token":"new-access-token","refresh_token":"new-refresh-token"}`))
	}))
	defer server.Close()
	restoreAPIBase := setFyersAPIBaseForTest(server.URL)
	defer restoreAPIBase()

	encryptedAccess := encryptForTest(t, "old-access-token")
	encryptedRefresh := encryptForTest(t, "old-refresh-token")
	encryptedPIN := encryptForTest(t, "1234")
	soon := time.Now().Add(1 * time.Hour)
	fs := &fakeStore{
		conn: &store.FyersConnection{
			EncryptedAccessToken:  encryptedAccess,
			EncryptedRefreshToken: encryptedRefresh,
			EncryptedPIN:          encryptedPIN,
			AccessTokenExpiresAt:  &soon,
			ConnectedBy:           strPtr("admin-1"),
		},
		users: map[string]*store.User{"admin-1": {ID: "admin-1", Email: "admin@example.com"}},
	}
	fp := &fakeProvider{}
	mail := &fakeMailer{}
	job := New(fs, fp, "APP", "SECRET", testCredKey, mail, nil, time.Minute, "https://example.com/admin/settings")

	job.tick(context.Background())

	if mail.reconnectCalls != 0 {
		t.Fatalf("expected no reconnect email on successful refresh, got %d", mail.reconnectCalls)
	}
	if fs.upsertCalls != 1 {
		t.Fatalf("expected exactly one token upsert, got %d", fs.upsertCalls)
	}
	if len(fp.tokens) == 0 || fp.tokens[len(fp.tokens)-1] != "new-access-token" {
		t.Fatalf("expected provider to receive the new access token, got %v", fp.tokens)
	}
}

func TestTick_RefreshFails_NotifiesReconnect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"s":"error","message":"refresh token expired"}`))
	}))
	defer server.Close()
	restoreAPIBase := setFyersAPIBaseForTest(server.URL)
	defer restoreAPIBase()

	encryptedAccess := encryptForTest(t, "old-access-token")
	encryptedRefresh := encryptForTest(t, "old-refresh-token")
	encryptedPIN := encryptForTest(t, "1234")
	soon := time.Now().Add(1 * time.Hour)
	fs := &fakeStore{
		conn: &store.FyersConnection{
			EncryptedAccessToken:  encryptedAccess,
			EncryptedRefreshToken: encryptedRefresh,
			EncryptedPIN:          encryptedPIN,
			AccessTokenExpiresAt:  &soon,
			ConnectedBy:           strPtr("admin-1"),
		},
		users: map[string]*store.User{"admin-1": {ID: "admin-1", Email: "admin@example.com"}},
	}
	fp := &fakeProvider{}
	mail := &fakeMailer{}
	job := New(fs, fp, "APP", "SECRET", testCredKey, mail, nil, time.Minute, "https://example.com/admin/settings")

	job.tick(context.Background())

	if mail.reconnectCalls != 1 {
		t.Fatalf("expected exactly one reconnect email after refresh failure, got %d", mail.reconnectCalls)
	}
	if fs.upsertCalls != 0 {
		t.Fatalf("expected no token upsert after a failed refresh, got %d", fs.upsertCalls)
	}
}

func strPtr(s string) *string { return &s }

func setFyersAPIBaseForTest(url string) (restore func()) {
	return fyersauth.SetAPIBaseForTest(url)
}
