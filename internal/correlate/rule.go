// Package correlate, trip'lerden alarm ureten korelasyon kurallarini icerir.
//
// Alert JSON semasi donmus wire contract'tir (PROJECT PLAN.md bol. 4);
// alanlar migrations/00001_init.sql icindeki alerts tablosuyla birebir
// hizalidir. Bu katmanda bilincli olarak ML/anomali yoktur: kurallar
// deterministiktir, sifir-FP tezi ancak boyle savunulur (bol. 2).
package correlate

import (
	"sort"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// Severity ve status degerleri migrations/00001_init.sql CHECK
// kisitlariyla birebir ayni tutulur.
const (
	SeverityHigh     = "High"
	SeverityCritical = "Critical"

	StatusOpen   = "open"
	StatusAck    = "ack"
	StatusClosed = "closed"
)

// SubjectAlerts, opsiyonel alarm yayin subject'idir (PLAN bol. 4).
const SubjectAlerts = "alerts.v1"

// TechniqueValidAccounts: canary credential kullanimi MITRE ATT&CK
// T1078 (Valid Accounts) teknigine eslesir (APP-11).
const TechniqueValidAccounts = "T1078"

// Alert, SOC paneline ve SIEM'e akan alarm kaydidir (alerts tablosuyla hizali).
type Alert struct {
	ID        string    `json:"id,omitempty"`
	Severity  string    `json:"severity"`
	Technique string    `json:"technique,omitempty"`
	Source    string    `json:"source"`
	Status    string    `json:"status"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	TripCount int       `json:"trip_count"`
}

// Evaluate, verilen zaman penceresindeki trip'leri kaynaga (Source) gore
// gruplayip kaynak basina tek alarm uretir:
//
//   - 1 trip   -> High
//   - >=2 trip -> tek Critical (kampanya birlesmesi)
//
// FirstSeen/LastSeen gruptaki en erken/en gec ObservedAt'tir. Cikti
// kaynak adina gore siralidir (deterministik). Zaman penceresini secmek
// ve alarmlari persist etmek cagiranin sorumlulugudur (APP-7).
func Evaluate(trips []trap.TripEvent) []Alert {
	if len(trips) == 0 {
		return nil
	}

	bySource := make(map[string][]trap.TripEvent)
	for _, tr := range trips {
		bySource[tr.Source] = append(bySource[tr.Source], tr)
	}

	sources := make([]string, 0, len(bySource))
	for src := range bySource {
		sources = append(sources, src)
	}
	sort.Strings(sources)

	alerts := make([]Alert, 0, len(sources))
	for _, src := range sources {
		group := bySource[src]

		first, last := group[0].ObservedAt, group[0].ObservedAt
		for _, tr := range group[1:] {
			if tr.ObservedAt.Before(first) {
				first = tr.ObservedAt
			}
			if tr.ObservedAt.After(last) {
				last = tr.ObservedAt
			}
		}

		severity := SeverityHigh
		if len(group) >= 2 {
			severity = SeverityCritical
		}

		alerts = append(alerts, Alert{
			Severity:  severity,
			Technique: TechniqueValidAccounts,
			Source:    src,
			Status:    StatusOpen,
			FirstSeen: first,
			LastSeen:  last,
			TripCount: len(group),
		})
	}

	return alerts
}
