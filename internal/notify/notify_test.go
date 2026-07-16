package notify

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
)

type fakeChannel struct {
	name string
	err  error

	mu    sync.Mutex
	calls []correlate.Alert
}

func (f *fakeChannel) Name() string { return f.name }

func (f *fakeChannel) Send(_ context.Context, a correlate.Alert) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, a)
	return f.err
}

func (f *fakeChannel) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func testLogger() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestFanout_NotifiesAllChannels(t *testing.T) {
	ok1 := &fakeChannel{name: "ok1"}
	ok2 := &fakeChannel{name: "ok2"}
	f := New(testLogger(), ok1, ok2)

	f.Notify(context.Background(), correlate.Alert{ID: "a1", Source: "10.0.0.1"})

	if ok1.callCount() != 1 || ok2.callCount() != 1 {
		t.Fatalf("beklenen: her iki kanal da 1 kez cagrilir; ok1=%d ok2=%d", ok1.callCount(), ok2.callCount())
	}
}

func TestFanout_OneChannelErrorDoesNotBlockOthers(t *testing.T) {
	failing := &fakeChannel{name: "failing", err: errors.New("baglanti hatasi")}
	ok := &fakeChannel{name: "ok"}
	f := New(testLogger(), failing, ok)

	f.Notify(context.Background(), correlate.Alert{ID: "a1", Source: "10.0.0.1"})

	if failing.callCount() != 1 {
		t.Errorf("basarisiz kanal yine de cagrilmali, cagri sayisi = %d", failing.callCount())
	}
	if ok.callCount() != 1 {
		t.Errorf("basarisiz kanal digerini bloklamamali, ok kanal cagri sayisi = %d", ok.callCount())
	}
}

func TestFanout_NoChannelsIsNoop(t *testing.T) {
	f := New(testLogger())
	f.Notify(context.Background(), correlate.Alert{ID: "a1"})
}
