package trap

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// usernamePrefix, provision edilen canary kullanicilarinin on ekidir.
// Demo ve tespit icin sabit tutulur (PLAN APP-2 AC: username `svc_` ile baslar).
const usernamePrefix = "svc_"

// CredentialCanaryProvider, credential_canary tuzagini uretir: rastgele bir
// `svc_` kullanicisi + rastgele bir secret. Secret duz metin olarak yalnizca
// Provision cevabinda (Artifacts) bir kez doner; kalici olarak sadece HMAC'i
// saklanir (secret_hash), boylece DB sizsa bile secret geri elde edilemez.
type CredentialCanaryProvider struct {
	hmacKey []byte
}

// NewCredentialCanaryProvider, verilen HMAC anahtariyla bir provider olusturur.
// Anahtar en az 32 bayt olmali (cagiran config katmani dogrular).
func NewCredentialCanaryProvider(hmacKey []byte) *CredentialCanaryProvider {
	return &CredentialCanaryProvider{hmacKey: hmacKey}
}

// Provision, yeni bir canary uretir. Donen Trap henuz persist edilmemistir
// (ID/CreatedAt bos); kalici kayit ve bu alanlarin doldurulmasi store katmaninin
// isidir. ctx su an kullanilmiyor ama Provider sozlesmesinin parcasi.
func (p *CredentialCanaryProvider) Provision(_ context.Context, createdBy string) (*Trap, *Artifacts, error) {
	if len(p.hmacKey) == 0 {
		return nil, nil, fmt.Errorf("trap: HMAC anahtari bos, canary secret'i imzalanamaz")
	}

	username, err := randomUsername()
	if err != nil {
		return nil, nil, err
	}
	secret, err := randomSecret()
	if err != nil {
		return nil, nil, err
	}

	t := &Trap{
		Type:       TypeCredentialCanary,
		Username:   username,
		SecretHash: p.hash(secret),
		CreatedBy:  createdBy,
	}
	return t, &Artifacts{Username: username, Secret: secret}, nil
}

// hash, secret'in HMAC-SHA256 ozetini hex olarak dondurur.
func (p *CredentialCanaryProvider) hash(secret string) string {
	mac := hmac.New(sha256.New, p.hmacKey)
	mac.Write([]byte(secret))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomUsername() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("trap: username uretilemedi: %w", err)
	}
	return usernamePrefix + hex.EncodeToString(b), nil
}

func randomSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("trap: secret uretilemedi: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
