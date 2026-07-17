package sensorssh

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// CanaryResolver, control-api'den tuzak listesini periyodik olarak ceker ve
// username -> trap_id eslemesini bellekte tutar. CanaryLookup'i karsilar,
// boylece Decoder hangi kullanicilarin canary oldugunu bilir.
type CanaryResolver struct {
	apiURL string
	client *http.Client
	logger *slog.Logger

	mu     sync.RWMutex
	byUser map[string]string
}

// NewCanaryResolver, control-api taban URL'i ile bir resolver olusturur.
func NewCanaryResolver(apiURL string, logger *slog.Logger) *CanaryResolver {
	return &CanaryResolver{
		apiURL: strings.TrimRight(apiURL, "/"),
		client: &http.Client{Timeout: 10 * time.Second},
		logger: logger,
		byUser: map[string]string{},
	}
}

// TrapID, bir kullanicinin trap_id'sini dondurur; kullanici bilinen bir canary
// degilse ok=false.
func (r *CanaryResolver) TrapID(username string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byUser[username]
	return id, ok
}

// Refresh, GET /api/v1/traps ile guncel eslemeyi kurar. Basarisizsa mevcut
// esleme korunur (bosaltilmaz) ve hata dondurulur — gecici bir API kesintisi
// sensoru "kor" birakmamali.
func (r *CanaryResolver) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.apiURL+"/api/v1/traps", nil)
	if err != nil {
		return fmt.Errorf("resolver: istek olusturulamadi: %w", err)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("resolver: control-api'ye ulasilamadi: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("resolver: beklenmeyen durum kodu: %d", resp.StatusCode)
	}

	var traps []trap.Trap
	if err := json.NewDecoder(resp.Body).Decode(&traps); err != nil {
		return fmt.Errorf("resolver: cevap cozulemedi: %w", err)
	}

	next := make(map[string]string, len(traps))
	for _, t := range traps {
		if t.RevokedAt != nil {
			continue // iptal edilmis tuzak artik trip uretmemeli
		}
		if t.Username != "" && t.ID != "" {
			next[t.Username] = t.ID
		}
	}

	r.mu.Lock()
	r.byUser = next
	r.mu.Unlock()
	return nil
}

// Run, interval araliklarinda Refresh cagirir; ctx iptal edilene kadar bloklar.
func (r *CanaryResolver) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.Refresh(ctx); err != nil {
				r.logger.Warn("canary listesi yenilenemedi", "err", err)
			}
		}
	}
}
