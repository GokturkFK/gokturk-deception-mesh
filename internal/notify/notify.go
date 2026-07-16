// Package notify, yeni/guncellenen bir alarmi Telegram + genel webhook +
// syslog CEF (SIEM) kanallarina dagitir (APP-8). Bir kanalin hatasi
// digerlerini bloklamaz: hepsi ayni anda (goroutine ile) gonderilir,
// basarisiz olan sadece loglanir (PLAN APP-8 AC: "kanal hatasi digerlerini bloklamiyor").
package notify

import (
	"context"
	"log/slog"
	"sync"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
)

// Channel, tek bir bildirim kanalidir (Telegram, webhook, syslog CEF, ...).
type Channel interface {
	Name() string
	Send(ctx context.Context, a correlate.Alert) error
}

// Fanout, bir alarmi tum kanallara ayni anda gonderir.
type Fanout struct {
	channels []Channel
	logger   *slog.Logger
}

// New, verilen kanallarla bir Fanout olusturur.
//
// DIKKAT: cagiran taraf, yapilandirilmamis kanallari (orn.
// NewTelegramChannel'in dondurdugu tipli nil *TelegramChannel) buraya
// gecirmeden ONCE elemelidir — bir nil pointer'i Channel arayuzune sarmak
// Go'da nil OLMAYAN bir arayuz degeri uretir ("typed nil"), bu yuzden
// burada "c != nil" ile filtrelemek guvenilmezdir. Dogru kullanim icin
// bkz. cmd/control-api/main.go.
func New(logger *slog.Logger, channels ...Channel) *Fanout {
	return &Fanout{channels: channels, logger: logger}
}

// Notify, alarmi tum aktif kanallara ayni anda gonderir ve hepsinin
// bitmesini bekler. Hata dondurmez: bildirim, alarmin kalici yazilmasina
// gore bir yan etkidir ve cagirani (alerting.Engine) bloklamamalidir.
func (f *Fanout) Notify(ctx context.Context, a correlate.Alert) {
	var wg sync.WaitGroup
	for _, ch := range f.channels {
		wg.Add(1)
		go func(ch Channel) {
			defer wg.Done()
			if err := ch.Send(ctx, a); err != nil {
				f.logger.Error("bildirim kanali basarisiz",
					"channel", ch.Name(), "alert_id", a.ID, "source", a.Source, "err", err)
			}
		}(ch)
	}
	wg.Wait()
}
