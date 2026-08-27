package auth

import (
	"encoding/base64"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	// 32バイトのダミー鍵をbase64で用意し、DecodeEncryptionKey経由で取得する
	// （実運用の`openssl rand -base64 32`と同じ経路を通す）。
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	key, err := DecodeEncryptionKey(encoded)
	if err != nil {
		t.Fatalf("DecodeEncryptionKey: %v", err)
	}
	return key
}

func TestDecodeEncryptionKey_WrongLengthRejected(t *testing.T) {
	shortKey := base64.StdEncoding.EncodeToString([]byte("too-short"))
	if _, err := DecodeEncryptionKey(shortKey); err == nil {
		t.Error("expected error for a key that does not decode to 32 bytes, got nil")
	}
}

func TestDecodeEncryptionKey_InvalidBase64Rejected(t *testing.T) {
	if _, err := DecodeEncryptionKey("not valid base64!!"); err == nil {
		t.Error("expected error for invalid base64 input, got nil")
	}
}

func TestEncryptDecryptToken_RoundTrip(t *testing.T) {
	key := testKey(t)
	plaintext := "1//0gTestRefreshToken-example"

	encrypted, err := EncryptToken(key, plaintext)
	if err != nil {
		t.Fatalf("EncryptToken: %v", err)
	}
	if encrypted == plaintext {
		t.Fatal("EncryptToken returned the plaintext unchanged")
	}

	decrypted, err := DecryptToken(key, encrypted)
	if err != nil {
		t.Fatalf("DecryptToken: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptToken_ProducesDifferentCiphertextEachTime(t *testing.T) {
	key := testKey(t)
	a, err := EncryptToken(key, "same-plaintext")
	if err != nil {
		t.Fatalf("EncryptToken: %v", err)
	}
	b, err := EncryptToken(key, "same-plaintext")
	if err != nil {
		t.Fatalf("EncryptToken: %v", err)
	}
	// nonceがランダムなため、同じ平文でも暗号文は毎回変わるはず（GCMの前提）。
	if a == b {
		t.Error("expected different ciphertexts for repeated encryption of the same plaintext (nonce reuse?)")
	}
}

func TestDecryptToken_TamperedCiphertextRejected(t *testing.T) {
	key := testKey(t)
	encrypted, err := EncryptToken(key, "secret-value")
	if err != nil {
		t.Fatalf("EncryptToken: %v", err)
	}

	raw, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("decode encrypted: %v", err)
	}
	raw[len(raw)-1] ^= 0xFF // 末尾1バイトを反転させ改ざんを模す
	tampered := base64.StdEncoding.EncodeToString(raw)

	if _, err := DecryptToken(key, tampered); err == nil {
		t.Error("expected error when decrypting tampered ciphertext, got nil")
	}
}

func TestDecryptToken_WrongKeyRejected(t *testing.T) {
	key := testKey(t)
	encrypted, err := EncryptToken(key, "secret-value")
	if err != nil {
		t.Fatalf("EncryptToken: %v", err)
	}

	wrongRaw := make([]byte, 32)
	for i := range wrongRaw {
		wrongRaw[i] = byte(255 - i)
	}

	if _, err := DecryptToken(wrongRaw, encrypted); err == nil {
		t.Error("expected error when decrypting with the wrong key, got nil")
	}
}
