package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer, gercek routing + auth middleware'ini (newServer) verilen
// token'larla kurar; boylece testler HTTP katmanindan uctan uca gecer.
func newTestServer(op, ro string, store trapStore) http.Handler {
	srv := newServer(config{httpAddr: ":0", operatorToken: op, readonlyToken: ro}, newTestAPI(store))
	return srv.Handler
}

func postTraps(h http.Handler, authHeader string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/traps", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAuth_Disabled_ProvisionOpen(t *testing.T) {
	// operatorToken bos → auth devre disi → token'siz provision gecer (201).
	h := newTestServer("", "", &fakeStore{})
	if rec := postTraps(h, ""); rec.Code != http.StatusCreated {
		t.Fatalf("auth kapaliyken status = %d, istenen 201", rec.Code)
	}
}

func TestAuth_Enabled_NoToken_401(t *testing.T) {
	h := newTestServer("op-secret", "ro-secret", &fakeStore{})
	rec := postTraps(h, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("token'siz provision status = %d, istenen 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Errorf("WWW-Authenticate = %q, istenen Bearer", got)
	}
}

func TestAuth_Enabled_WrongToken_401(t *testing.T) {
	h := newTestServer("op-secret", "ro-secret", &fakeStore{})
	if rec := postTraps(h, "Bearer yanlis"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("yanlis token status = %d, istenen 401", rec.Code)
	}
}

func TestAuth_Enabled_ReadonlyToken_403(t *testing.T) {
	// AC: read-only rol provision yapamaz.
	h := newTestServer("op-secret", "ro-secret", &fakeStore{})
	if rec := postTraps(h, "Bearer ro-secret"); rec.Code != http.StatusForbidden {
		t.Fatalf("read-only token ile provision status = %d, istenen 403", rec.Code)
	}
}

func TestAuth_Enabled_OperatorToken_201(t *testing.T) {
	fs := &fakeStore{}
	h := newTestServer("op-secret", "ro-secret", fs)
	if rec := postTraps(h, "Bearer op-secret"); rec.Code != http.StatusCreated {
		t.Fatalf("operator token ile provision status = %d, istenen 201", rec.Code)
	}
	if len(fs.created) != 1 {
		t.Errorf("operator provision store'a yazmali: %d kayit", len(fs.created))
	}
}

func TestAuth_Enabled_ReadIsOpen(t *testing.T) {
	// Tasarim: okumalar auth aciksa bile token istemez (ic cagiranlar kirilmaz).
	h := newTestServer("op-secret", "ro-secret", &fakeStore{})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/traps", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("token'siz GET /api/v1/traps status = %d, istenen 200", rec.Code)
	}
}

func TestBearerToken(t *testing.T) {
	cases := map[string]string{
		"Bearer abc":  "abc",
		"bearer abc":  "abc", // case-insensitive scheme
		"Bearer  abc": "abc", // fazla bosluk trim'lenir
		"Basic abc":   "",
		"abc":         "",
		"":            "",
		"Bearer":      "",
	}
	for header, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		if got := bearerToken(req); got != want {
			t.Errorf("bearerToken(%q) = %q, istenen %q", header, got, want)
		}
	}
}

func TestTokenMatch_EmptyWantNeverMatches(t *testing.T) {
	// READONLY_TOKEN ayarli degilken bos Bearer token read-only'ye dusmemeli.
	if tokenMatch("", "") {
		t.Error("bos want ile bos got eslesmemeli")
	}
	if !tokenMatch("x", "x") {
		t.Error("ayni token eslesmeli")
	}
}
