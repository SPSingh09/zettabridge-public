package credenc

import (
	"encoding/base64"
	"strings"
	"testing"
)

func testKey() []byte {
	key, err := ParseKey("a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566778899aabbccddeeff00")
	if err != nil {
		panic(err)
	}
	return key
}

func TestParseKey(t *testing.T) {
	key, err := ParseKey("a1b2c3d4e5f60718293a4b5c6d7e8f90112233445566778899aabbccddeeff00")
	if err != nil || len(key) != keySize {
		t.Fatalf("ParseKey: %v len=%d", err, len(key))
	}
	if _, err := ParseKey("short"); err == nil {
		t.Fatal("expected invalid key error")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key := testKey()
	credID := "cred-123"
	plain := "api_key:api_secret:token"

	blob, err := Encrypt(plain, key, credID)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !IsEncrypted(blob) {
		t.Fatalf("expected encrypted blob, got %q", blob)
	}

	got, err := Decrypt(blob, key, credID)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("round-trip mismatch: got %q want %q", got, plain)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key := testKey()
	blob, err := Encrypt("secret", key, "cred-a")
	if err != nil {
		t.Fatal(err)
	}
	other, _ := ParseKey("1111111111111111111111111111111111111111111111111111111111111111")
	if _, err := Decrypt(blob, other, "cred-a"); err == nil {
		t.Fatal("expected decrypt failure with wrong key")
	}
}

func TestDecryptWrongCredID(t *testing.T) {
	key := testKey()
	blob, err := Encrypt("secret", key, "cred-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(blob, key, "cred-b"); err == nil {
		t.Fatal("expected decrypt failure with wrong cred id")
	}
}

func TestDecryptTamperedCiphertext(t *testing.T) {
	key := testKey()
	blob, err := Encrypt("secret", key, "cred-a")
	if err != nil {
		t.Fatal(err)
	}
	tampered := blob[:len(blob)-2] + "AA"
	if _, err := Decrypt(tampered, key, "cred-a"); err == nil {
		t.Fatal("expected decrypt failure for tampered blob")
	}
}

func TestPlaintextOrDecryptLegacy(t *testing.T) {
	key := testKey()
	got, err := PlaintextOrDecrypt("api_key:secret", key, "cred-1")
	if err != nil || got != "api_key:secret" {
		t.Fatalf("legacy passthrough: got %q err=%v", got, err)
	}
}

func TestPlaintextOrDecryptEncrypted(t *testing.T) {
	key := testKey()
	blob, err := Encrypt("api_key:secret", key, "cred-1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := PlaintextOrDecrypt(blob, key, "cred-1")
	if err != nil || got != "api_key:secret" {
		t.Fatalf("decrypt via helper: got %q err=%v", got, err)
	}
}

func TestEncryptUniqueNonces(t *testing.T) {
	key := testKey()
	b1, err := Encrypt("same", key, "cred-1")
	if err != nil {
		t.Fatal(err)
	}
	b2, err := Encrypt("same", key, "cred-1")
	if err != nil {
		t.Fatal(err)
	}
	if b1 == b2 {
		t.Fatal("expected unique ciphertext for same plaintext")
	}
}

func TestEncryptRejectsEmptyPlaintext(t *testing.T) {
	if _, err := Encrypt("   ", testKey(), "cred-1"); err != ErrEmptyPlaintext {
		t.Fatalf("expected ErrEmptyPlaintext, got %v", err)
	}
}

func TestIsWeakKey(t *testing.T) {
	zero, _ := ParseKey(strings.Repeat("0", 64))
	if !IsWeakKey(zero) {
		t.Fatal("all-zero key should be weak")
	}
	if IsWeakKey(testKey()) {
		t.Fatal("random key should not be weak")
	}
}

func TestDecryptInvalidBlob(t *testing.T) {
	if _, err := Decrypt("not-encrypted", testKey(), "id"); err != ErrInvalidBlob {
		t.Fatalf("expected ErrInvalidBlob, got %v", err)
	}
	raw := versionPrefix + base64.StdEncoding.EncodeToString([]byte("ab"))
	if _, err := Decrypt(raw, testKey(), "id"); err != ErrInvalidBlob {
		t.Fatalf("expected ErrInvalidBlob for short payload, got %v", err)
	}
}
