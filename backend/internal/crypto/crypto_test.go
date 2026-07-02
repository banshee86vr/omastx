package crypto

import (
	"bytes"
	"strings"
	"testing"
)

func key(b byte) []byte {
	return bytes.Repeat([]byte{b}, 32)
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"kubeconfig-like", []byte("apiVersion: v1\nkind: Config\nclusters: []\n")},
		{"empty", []byte{}},
		{"binary", []byte{0x00, 0xff, 0x10, 0x80}},
		{"large", bytes.Repeat([]byte("x"), 1<<20)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct, nonce, err := Encrypt(key(1), tt.plaintext)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			if bytes.Contains(ct, tt.plaintext) && len(tt.plaintext) > 0 {
				t.Fatal("ciphertext contains plaintext")
			}
			got, err := Decrypt(key(1), ct, nonce)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if !bytes.Equal(got, tt.plaintext) {
				t.Errorf("roundtrip mismatch")
			}
		})
	}
}

func TestDecryptFailures(t *testing.T) {
	ct, nonce, err := Encrypt(key(1), []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("wrong key", func(t *testing.T) {
		if _, err := Decrypt(key(2), ct, nonce); err == nil {
			t.Error("expected error with wrong key")
		}
	})
	t.Run("tampered ciphertext", func(t *testing.T) {
		bad := append([]byte{}, ct...)
		bad[0] ^= 0x01
		if _, err := Decrypt(key(1), bad, nonce); err == nil {
			t.Error("expected error with tampered ciphertext")
		}
	})
	t.Run("wrong nonce", func(t *testing.T) {
		bad := append([]byte{}, nonce...)
		bad[0] ^= 0x01
		if _, err := Decrypt(key(1), ct, bad); err == nil {
			t.Error("expected error with wrong nonce")
		}
	})
}

func TestKeyLengthEnforced(t *testing.T) {
	for _, n := range []int{0, 16, 31, 33} {
		if _, _, err := Encrypt(bytes.Repeat([]byte{1}, n), []byte("x")); err == nil ||
			!strings.Contains(err.Error(), "32 bytes") {
			t.Errorf("key length %d: err = %v, want 32-bytes error", n, err)
		}
	}
}

func TestNoncesAreUnique(t *testing.T) {
	_, n1, err := Encrypt(key(1), []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	_, n2, err := Encrypt(key(1), []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(n1, n2) {
		t.Error("nonces must be random per encryption")
	}
}
