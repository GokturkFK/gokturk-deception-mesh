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

// TelegramChannel, alarmi bir Telegram bot'u uzerinden sohbete mesaj
// olarak gonderir (Bot API sendMessage).
type TelegramChannel struct {
	botToken string
	chatID   string
	client   *http.Client
	baseURL  string
}

// NewTelegramChannel, botToken veya chatID bos ise nil doner: kanal
// yapilandirilmamis demektir ve Fanout tarafindan sessizce atlanir.
func NewTelegramChannel(botToken, chatID string) *TelegramChannel {
	if botToken == "" || chatID == "" {
		return nil
	}
	return &TelegramChannel{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 5 * time.Second},
		baseURL:  "https://api.telegram.org",
	}
}

// Name, Fanout loglarinda kanali tanimlamak icin kullanilir.
func (t *TelegramChannel) Name() string { return "telegram" }

// Send, alarmi Telegram Bot API'sine gonderir.
func (t *TelegramChannel) Send(ctx context.Context, a correlate.Alert) error {
	text := fmt.Sprintf(
		"\U0001F6A8 [%s] Gokturk Deception Mesh\nKaynak: %s\nTeknik: %s\nDurum: %s\nTrip sayisi: %d\nIlk gorulme: %s",
		a.Severity, a.Source, a.Technique, a.Status, a.TripCount, a.FirstSeen.Format(time.RFC3339))

	body, err := json.Marshal(map[string]string{
		"chat_id": t.chatID,
		"text":    text,
	})
	if err != nil {
		return fmt.Errorf("notify: telegram govdesi kodlanamadi: %w", err)
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", t.baseURL, t.botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("notify: telegram istegi olusturulamadi: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("notify: telegram istegi basarisiz: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("notify: telegram beklenmeyen durum kodu: %d", resp.StatusCode)
	}
	return nil
}
