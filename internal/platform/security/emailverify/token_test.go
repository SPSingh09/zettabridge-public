package emailverify

import "testing"

func TestGenerateAndHash(t *testing.T) {
	raw, hash, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateFormat(raw) {
		t.Fatal("expected valid token format")
	}
	if Hash(raw) != hash {
		t.Fatal("hash mismatch")
	}
}

func TestValidateFormatRejectsGarbage(t *testing.T) {
	if ValidateFormat("not-a-token") {
		t.Fatal("expected invalid")
	}
}
