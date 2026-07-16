package notify

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
)

// severityToCEF, alarmin severity string'ini CEF'in 0-10 tamsayi onem
// olcegine esler (CEF spesifikasyonu: 0-3 Low, 4-6 Medium, 7-8 High,
// 9-10 Very-High). Bilinmeyen degerler Medium'a duser.
func severityToCEF(severity string) int {
	switch severity {
	case correlate.SeverityCritical:
		return 10
	case correlate.SeverityHigh:
		return 8
	default:
		return 5
	}
}

// SyslogCEFChannel, alarmi UDP uzerinden ArcSight CEF formatinda bir
// SIEM'e gonderir (bkz. deployments/docker/docker-compose.yml: siem-sink).
type SyslogCEFChannel struct {
	addr string // "host:port"
}

// NewSyslogCEFChannel, addr bos ise nil doner: kanal yapilandirilmamis
// demektir ve Fanout tarafindan sessizce atlanir.
func NewSyslogCEFChannel(addr string) *SyslogCEFChannel {
	if addr == "" {
		return nil
	}
	return &SyslogCEFChannel{addr: addr}
}

// Name, Fanout loglarinda kanali tanimlamak icin kullanilir.
func (s *SyslogCEFChannel) Name() string { return "syslog-cef" }

// Send, alarmi CEF formatinda tek bir UDP datagram olarak gonderir.
// UDP baglantisizdir; SIEM gecici olarak ayakta degilse Dial genelde
// basarili olur ama Write veri kaybedebilir — bu, demo SIEM alicisi icin
// kabul edilebilir bir tur (fire-and-forget, at-most-once).
func (s *SyslogCEFChannel) Send(_ context.Context, a correlate.Alert) error {
	conn, err := net.DialTimeout("udp", s.addr, 3*time.Second)
	if err != nil {
		return fmt.Errorf("notify: syslog baglantisi kurulamadi: %w", err)
	}
	defer func() { _ = conn.Close() }()

	cef := fmt.Sprintf(
		"CEF:0|GokturkFK|gokturk-deception-mesh|1.0|%s|%s|%d|src=%s cnt=%d cs1Label=status cs1=%s start=%s\n",
		a.Technique, a.Severity, severityToCEF(a.Severity), a.Source, a.TripCount, a.Status,
		a.FirstSeen.Format(time.RFC3339),
	)

	if _, err := conn.Write([]byte(cef)); err != nil {
		return fmt.Errorf("notify: syslog yazilamadi: %w", err)
	}
	return nil
}
