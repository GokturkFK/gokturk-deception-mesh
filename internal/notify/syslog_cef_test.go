package notify

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
)

func TestNewSyslogCEFChannel_EmptyAddrReturnsNil(t *testing.T) {
	if ch := NewSyslogCEFChannel(""); ch != nil {
		t.Error("bos addr ile nil donmeli")
	}
}

func TestSyslogCEFChannel_SendsWellFormedCEF(t *testing.T) {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("udp listener: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ch := NewSyslogCEFChannel(conn.LocalAddr().String())
	a := correlate.Alert{
		Severity: correlate.SeverityCritical, Technique: correlate.TechniqueValidAccounts,
		Source: "10.0.0.1", Status: correlate.StatusOpen, TripCount: 2, FirstSeen: time.Now().UTC(),
	}

	errCh := make(chan error, 1)
	go func() { errCh <- ch.Send(context.Background(), a) }()

	buf := make([]byte, 1024)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		t.Fatalf("udp okuma: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Send: %v", err)
	}

	msg := string(buf[:n])
	if !strings.HasPrefix(msg, "CEF:0|GokturkFK|gokturk-deception-mesh|1.0|") {
		t.Errorf("CEF header hatali: %q", msg)
	}
	if !strings.Contains(msg, "src=10.0.0.1") {
		t.Errorf("src alani eksik: %q", msg)
	}
	if !strings.Contains(msg, "cnt=2") {
		t.Errorf("cnt alani eksik: %q", msg)
	}
}

func TestSeverityToCEF(t *testing.T) {
	cases := map[string]int{
		correlate.SeverityCritical: 10,
		correlate.SeverityHigh:     8,
		"unknown":                  5,
	}
	for sev, want := range cases {
		if got := severityToCEF(sev); got != want {
			t.Errorf("severityToCEF(%q) = %d, istenen %d", sev, got, want)
		}
	}
}
