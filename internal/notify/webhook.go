package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
)

// WebhookChannel, alarmi JSON govde olarak genel bir HTTP endpoint'ine
// POST eder (SOAR/otomasyon entegrasyonlari icin).
type WebhookChannel struct {
	url    string
	client *http.Client
}

// NewWebhookChannel, url bos ise nil doner: kanal yapilandirilmamis
// demektir ve Fanout tarafindan sessizce atlanir.
func NewWebhookChannel(url string) *WebhookChannel {
	if url == "" {
		return nil
	}
	return &WebhookChannel{url: url, client: &http.Client{Timeout: 5 * time.Second}}
}

// Name, Fanout loglarinda kanali tanimlamak icin kullanilir.
func (w *WebhookChannel) Name() string { return "webhook" }

// Send, alarmin JSON gosterimini yapilandirilmis URL'e POST eder.
func (w *WebhookChannel) Send(ctx context.Context, a correlate.Alert) error {
	body, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("notify: webhook govdesi kodlanamadi: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notify: webhook istegi olusturulamadi: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("notify: webhook istegi basarisiz: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("notify: webhook beklenmeyen durum kodu: %d", resp.StatusCode)
	}
	return nil
}
