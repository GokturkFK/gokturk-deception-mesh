package main

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// authConfig, APP-3 token tabanli yetkilendirmeyi tutar. Iki rol vardir:
//
//	operator  — mutasyon (tuzak provision) dahil her sey.
//	read-only — yalnizca okuma; provision denerse 403 alir.
//
// Tasarim kararı (v0.1, bkz. docs/THREAT_MODEL.md STRIDE > EoP): yalnizca
// MUTASYON endpoint'i (POST /api/v1/traps) kilitlenir. GET okumalari (traps,
// alerts) acik birakilir — bunlar secret sizdirmaz (secret_hash json:"-") ve
// icerideki cagiranlar (sensor resolver, dashboard) yalnizca GET yapar, yani
// auth acilinca kirilmazlar. Okuma tarafinin kilitlenmesi Sprint 3 mTLS'e
// (OPS-9) birakildi.
//
// Geriye donuk uyumluluk: operatorToken bos ise auth tamamen devre disidir
// (air-gapped/gelistirme icin). Bu durumda mevcut davranis birebir korunur.
type authConfig struct {
	operatorToken string
	readonlyToken string
}

// enabled, auth'un yapilandirilip yapilandirilmadigini soyler.
func (ac authConfig) enabled() bool { return ac.operatorToken != "" }

// bearerToken, "Authorization: Bearer <token>" header'indan token'i cikarir;
// yoksa bos string doner.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// tokenMatch, sabit-zamanli karsilastirma yapar. Yapilandirilmis taraf (want)
// bos ise asla eslesmez — boylece "READONLY_TOKEN ayarli degil" durumunda bos
// bir Bearer token'i read-only rolune dusmez.
func tokenMatch(got, want string) bool {
	if want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// requireOperator, bir handler'i operator token'ina kilitler:
//   - auth devre disi (operatorToken bos) → dogrudan gecer.
//   - operator token → gecer.
//   - gecerli read-only token → 403 (rol provision yapamaz).
//   - eksik/gecersiz token → 401.
func (ac authConfig) requireOperator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ac.enabled() {
			next(w, r)
			return
		}
		tok := bearerToken(r)
		switch {
		case tokenMatch(tok, ac.operatorToken):
			next(w, r)
		case tokenMatch(tok, ac.readonlyToken):
			writeError(w, http.StatusForbidden, "bu islem icin operator yetkisi gerekir")
		default:
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "gecerli bir yetki anahtari gerekir")
		}
	}
}
