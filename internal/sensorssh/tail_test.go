package sensorssh

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// collectTail, TailFile'i bir goroutine'de calistirir ve okunan satirlari kanala akitir.
// Test bitince ctx iptal edilir.
func collectTail(t *testing.T, path string, fromStart bool) (<-chan string, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	lines := make(chan string, 64)
	go func() {
		_ = TailFile(ctx, path, 10*time.Millisecond, fromStart, func(l string) {
			select {
			case lines <- l:
			case <-ctx.Done():
			}
		})
	}()
	return lines, cancel
}

func waitLine(t *testing.T, lines <-chan string, want string) {
	t.Helper()
	select {
	case got := <-lines:
		if got != want {
			t.Fatalf("satir = %q, istenen %q", got, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("satir %q icin zaman asimi", want)
	}
}

func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("dosya acilamadi: %v", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("yazilamadi: %v", err)
	}
}

func TestTailFile_FromStartAndFollow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.log")
	appendLine(t, path, "satir-1")

	lines, cancel := collectTail(t, path, true)
	defer cancel()

	waitLine(t, lines, "satir-1")

	// Canli olarak eklenen satirlar da gelmeli.
	appendLine(t, path, "satir-2")
	waitLine(t, lines, "satir-2")
}

func TestTailFile_FromEndSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.log")
	appendLine(t, path, "eski-satir")

	lines, cancel := collectTail(t, path, false)
	defer cancel()

	// fromStart=false: eski icerik atlanir. Poll'un baslamasina biraz zaman taniyip
	// yeni satir ekle; yalnizca yeni satir gelmeli.
	time.Sleep(50 * time.Millisecond)
	appendLine(t, path, "yeni-satir")
	waitLine(t, lines, "yeni-satir")
}

func TestTailFile_WaitsForMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gelecek.log")

	lines, cancel := collectTail(t, path, true)
	defer cancel()

	// Dosya sonradan olusuyor; tailer onu bekleyip okumali.
	time.Sleep(50 * time.Millisecond)
	appendLine(t, path, "gec-gelen")
	waitLine(t, lines, "gec-gelen")
}

func TestTailFile_Rotation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("acik dosya rename Windows'ta paylasim ihlali verir; rotasyon Linux CI'da dogrulanir")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.log")
	appendLine(t, path, "rotate-oncesi")

	lines, cancel := collectTail(t, path, true)
	defer cancel()
	waitLine(t, lines, "rotate-oncesi")

	// Rotate: mevcut dosyayi tasit, ayni yola yeni bir dosya koy.
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	appendLine(t, path, "rotate-sonrasi")
	waitLine(t, lines, "rotate-sonrasi")
}
