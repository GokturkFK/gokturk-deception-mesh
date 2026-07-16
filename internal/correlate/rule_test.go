package correlate

import (
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

func trip(source string, at time.Time) trap.TripEvent {
	return trap.TripEvent{
		EventID:    "evt-" + source + "-" + at.Format(time.RFC3339),
		TrapID:     "trap-1",
		Sensor:     "sensor-ssh",
		Source:     source,
		ObservedAt: at,
	}
}

func TestEvaluate(t *testing.T) {
	t0 := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	t1 := t0.Add(1 * time.Minute)
	t2 := t0.Add(2 * time.Minute)

	cases := []struct {
		name  string
		trips []trap.TripEvent
		want  []Alert
	}{
		{
			name:  "bos girdi -> alarm yok",
			trips: nil,
			want:  nil,
		},
		{
			name:  "tek trip -> 1 High",
			trips: []trap.TripEvent{trip("10.0.0.1", t0)},
			want: []Alert{{
				Severity: SeverityHigh, Technique: TechniqueValidAccounts,
				Source: "10.0.0.1", Status: StatusOpen,
				FirstSeen: t0, LastSeen: t0, TripCount: 1,
			}},
		},
		{
			name: "ayni kaynaktan 2 trip -> tek Critical (kampanya birlesmesi)",
			// Kasitli olarak ters zaman sirasiyla: first/last siradan bagimsiz dogru olmali.
			trips: []trap.TripEvent{trip("10.0.0.1", t2), trip("10.0.0.1", t0)},
			want: []Alert{{
				Severity: SeverityCritical, Technique: TechniqueValidAccounts,
				Source: "10.0.0.1", Status: StatusOpen,
				FirstSeen: t0, LastSeen: t2, TripCount: 2,
			}},
		},
		{
			name: "farkli kaynaklar -> kaynak basina ayri alarm, kaynak adina gore sirali",
			trips: []trap.TripEvent{
				trip("10.0.0.2", t1),
				trip("10.0.0.1", t0),
				trip("10.0.0.2", t2),
			},
			want: []Alert{
				{
					Severity: SeverityHigh, Technique: TechniqueValidAccounts,
					Source: "10.0.0.1", Status: StatusOpen,
					FirstSeen: t0, LastSeen: t0, TripCount: 1,
				},
				{
					Severity: SeverityCritical, Technique: TechniqueValidAccounts,
					Source: "10.0.0.2", Status: StatusOpen,
					FirstSeen: t1, LastSeen: t2, TripCount: 2,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(tc.trips)
			if len(got) != len(tc.want) {
				t.Fatalf("alarm sayisi = %d, istenen %d (cikti: %+v)", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				w, g := tc.want[i], got[i]
				if g.Severity != w.Severity || g.Technique != w.Technique ||
					g.Source != w.Source || g.Status != w.Status ||
					!g.FirstSeen.Equal(w.FirstSeen) || !g.LastSeen.Equal(w.LastSeen) ||
					g.TripCount != w.TripCount {
					t.Errorf("alert[%d] = %+v, istenen %+v", i, g, w)
				}
			}
		})
	}
}
