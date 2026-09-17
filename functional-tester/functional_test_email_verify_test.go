//go:build functional

package functionaltester

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestEmailVerificationFlow(t *testing.T) {
	if os.Getenv("FUNCTIONAL_TEST_EMAIL_VERIFY") != "1" {
		t.Skip("set FUNCTIONAL_TEST_EMAIL_VERIFY=1 with server EMAIL_VERIFICATION_REQUIRED=true (see docs/plans/email-verification.md)")
	}

	s := newState()
	email := uniqueEmail("verify")
	password := "verify-pass-123"

	type registerReq struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	type registerResp struct {
		ID                          string `json:"id"`
		Email                       string `json:"email"`
		EmailVerificationRequired   bool   `json:"email_verification_required"`
	}

	var reg registerResp
	callJSON(s, t, http.MethodPost, "/v1/auth/register", "", registerReq{Email: email, Password: password}, http.StatusCreated, &reg)
	if !reg.EmailVerificationRequired {
		t.Fatal("expected email_verification_required true")
	}

	type loginReq struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	status, raw := s.request(t, http.MethodPost, "/v1/auth/login", "", loginReq{Email: email, Password: password})
	if status != http.StatusForbidden {
		t.Fatalf("login before verify: want 403 got %d body=%s", status, trimBody(raw))
	}
	if !strings.Contains(string(raw), "email not verified") {
		t.Fatalf("expected email not verified error, body=%s", trimBody(raw))
	}

	// Operator copies verify URL from server logs (EMAIL_PROVIDER=log).
	token := os.Getenv("FUNCTIONAL_TEST_VERIFY_TOKEN")
	if token == "" {
		t.Skip("set FUNCTIONAL_TEST_VERIFY_TOKEN from server log (email_verify: url=...)")
	}

	type verifyResp struct {
		Verified bool   `json:"verified"`
		Email    string `json:"email"`
	}
	var verified verifyResp
	callJSON(s, t, http.MethodGet, "/v1/auth/verify-email?token="+token, "", nil, http.StatusOK, &verified)
	if !verified.Verified || verified.Email != strings.ToLower(email) {
		t.Fatalf("verify response: %#v", verified)
	}

	type loginResp struct {
		Token string `json:"token"`
	}
	var login loginResp
	callJSON(s, t, http.MethodPost, "/v1/auth/login", "", loginReq{Email: email, Password: password}, http.StatusOK, &login)
	if login.Token == "" {
		t.Fatal("expected JWT after verification")
	}
}
