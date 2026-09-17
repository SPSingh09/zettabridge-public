package brokercreds

import "testing"

func TestParseZerodhaPublisher(t *testing.T) {
	// 1-field: api_key only (publisher mode)
	p, err := Parse("zerodha", "myapikey")
	if err != nil {
		t.Fatal(err)
	}
	if p.APIKey != "myapikey" || p.APISecret != "" || p.AccessToken != "" {
		t.Fatalf("unexpected: %#v", p)
	}
}

func TestParseZerodha(t *testing.T) {
	// 2-field: api_key:api_secret (pre-OAuth)
	p, err := Parse("zerodha", "abc123:secret456")
	if err != nil {
		t.Fatal(err)
	}
	if p.APIKey != "abc123" || p.APISecret != "secret456" || p.AccessToken != "" {
		t.Fatalf("unexpected: %#v", p)
	}
}

func TestParseZerodha3Field(t *testing.T) {
	// 3-field: api_key:api_secret:access_token (post-OAuth)
	p, err := Parse("zerodha", "abc123:secret456:token789")
	if err != nil {
		t.Fatal(err)
	}
	if p.APIKey != "abc123" || p.APISecret != "secret456" || p.AccessToken != "token789" {
		t.Fatalf("unexpected: %#v", p)
	}
}

func TestParseZerodha4FieldRejected(t *testing.T) {
	// 4-field: should be rejected
	if _, err := Parse("zerodha", "a:b:c:d"); err == nil {
		t.Fatal("expected error for 4-part zerodha creds")
	}
}

func TestParseMT5(t *testing.T) {
	p, err := Parse("mt5_cloud", "tok:acc-1")
	if err != nil || p.AuthToken != "tok" || p.AccountID != "acc-1" {
		t.Fatalf("got %#v err=%v", p, err)
	}
}

func TestParseAngel(t *testing.T) {
	_, err := Parse("angel", "a:b")
	if err == nil {
		t.Fatal("expected error for short angel creds")
	}
	p, err := Parse("angel", "key:client:jwt")
	if err != nil {
		t.Fatal(err)
	}
	if p.JWT != "jwt" {
		t.Fatalf("got %#v", p)
	}
}

func TestParseRejectsEmptyParts(t *testing.T) {
	if _, err := Parse("zerodha", "key:"); err == nil {
		t.Fatal("expected error")
	}
}
