package trap

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestCredentialCanaryProvider_Provision(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	p := NewCredentialCanaryProvider(key)

	tr, art, err := p.Provision(context.Background(), "tester")
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}

	if !strings.HasPrefix(tr.Username, "svc_") {
		t.Errorf("username %q, svc_ ile baslamali", tr.Username)
	}
	if tr.Username != art.Username {
		t.Errorf("Trap.Username (%q) ile Artifacts.Username (%q) eslesmiyor", tr.Username, art.Username)
	}
	if tr.Type != TypeCredentialCanary {
		t.Errorf("type = %q, istenen %q", tr.Type, TypeCredentialCanary)
	}
	if tr.CreatedBy != "tester" {
		t.Errorf("createdBy = %q", tr.CreatedBy)
	}
	if art.Secret == "" {
		t.Error("secret bos olmamali")
	}
	if tr.SecretHash == art.Secret {
		t.Error("secret_hash duz secret'e esit olmamali (hash'lenmeli)")
	}

	// secret_hash gercekten HMAC-SHA256(key, secret) olmali.
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(art.Secret))
	want := hex.EncodeToString(mac.Sum(nil))
	if tr.SecretHash != want {
		t.Errorf("secret_hash = %q, istenen %q", tr.SecretHash, want)
	}
}

func TestCredentialCanaryProvider_Uniqueness(t *testing.T) {
	p := NewCredentialCanaryProvider([]byte("0123456789abcdef0123456789abcdef"))

	seenUser := map[string]bool{}
	seenSecret := map[string]bool{}
	for i := 0; i < 100; i++ {
		tr, art, err := p.Provision(context.Background(), "x")
		if err != nil {
			t.Fatalf("beklenmeyen hata: %v", err)
		}
		if seenUser[tr.Username] {
			t.Fatalf("username tekrar etti: %q", tr.Username)
		}
		if seenSecret[art.Secret] {
			t.Fatalf("secret tekrar etti")
		}
		seenUser[tr.Username] = true
		seenSecret[art.Secret] = true
	}
}

func TestCredentialCanaryProvider_EmptyKey(t *testing.T) {
	p := NewCredentialCanaryProvider(nil)
	if _, _, err := p.Provision(context.Background(), "x"); err == nil {
		t.Fatal("bos HMAC anahtariyla Provision hata dondurmeliydi")
	}
}
