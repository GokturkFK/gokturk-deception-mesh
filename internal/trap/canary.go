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

	// usernameGen, nil ise varsayilan `svc_`+hex uretimi kullanilir.
	// APP-12 seeding'de profil tabanli, "inandirici" ad uretimiyle degistirilir.
	usernameGen func() (string, error)
}

// CanaryOption, provider davranisini degistiren opsiyonel ayardir.
type CanaryOption func(*CredentialCanaryProvider)

// WithUsernameGenerator, kullanici adi uretimini disaridan verilen fonksiyonla
// degistirir (APP-12).
//
// Gerekce: varsayilan `svc_`+hex adi bir hedefe EKILDIGINDE sirittir (tek tip
// on ek + yuksek entropili blok). Seeding yolunda internal/seed profilleri bu
// opsiyonla devreye girer; varsayilan yol (dogrudan API provision) degismez,
// boylece APP-2 davranisi ve mevcut demo aynen korunur.
func WithUsernameGenerator(gen func() (string, error)) CanaryOption {
	return func(p *CredentialCanaryProvider) { p.usernameGen = gen }
}

// NewCredentialCanaryProvider, verilen HMAC anahtariyla bir provider olusturur.
// Anahtar en az 32 bayt olmali (cagiran config katmani dogrular).
func NewCredentialCanaryProvider(hmacKey []byte, opts ...CanaryOption) *CredentialCanaryProvider {
	p := &CredentialCanaryProvider{hmacKey: hmacKey}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Provision, yeni bir canary uretir. Donen Trap henuz persist edilmemistir
// (ID/CreatedAt bos); kalici kayit ve bu alanlarin doldurulmasi store katmaninin
// isidir. ctx su an kullanilmiyor ama Provider sozlesmesinin parcasi.
func (p *CredentialCanaryProvider) Provision(_ context.Context, createdBy string) (*Trap, *Artifacts, error) {
	if len(p.hmacKey) == 0 {
		return nil, nil, fmt.Errorf("trap: HMAC anahtari bos, canary secret'i imzalanamaz")
	}

	gen := p.usernameGen
	if gen == nil {
		gen = randomUsername
	}
	username, err := gen()
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
