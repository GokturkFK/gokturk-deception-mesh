package main

import (
	"context"
	"net/http"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/correlate"
)

// alertLister, dashboard'un (APP-9) ihtiyac duydugu kalici katman
// sozlesmesidir. Arayuz olarak tutulur ki handler DB olmadan (fake ile)
// test edilebilsin.
type alertLister interface {
	ListAlerts(ctx context.Context) ([]correlate.Alert, error)
}

// handleListAlerts, GET /api/v1/alerts — alarmlari en son guncellenen
// once olacak sekilde listeler (Streamlit dashboard bunu periyodik ceker).
func (a *apiServer) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, err := a.alerts.ListAlerts(r.Context())
	if err != nil {
		a.logger.Error("alarmlar listelenemedi", "err", err)
		writeError(w, http.StatusInternalServerError, "alarmlar listelenemedi")
		return
	}
	if alerts == nil {
		alerts = []correlate.Alert{}
	}
	writeJSON(w, http.StatusOK, alerts)
}
