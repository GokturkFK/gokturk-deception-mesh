// Package trap, tuzak (deception) cekirdeginin donmus sozlesmelerini tanimlar.
//
// TripEvent JSON semasi degismez wire contract'tir (PROJECT PLAN.md bol. 4).
// Sema degisikligi ancak subject version bump (trip.events.v2) ile yapilir
// (bol. 10, kontrat kaymasi riski). Alanlar migrations/00001_init.sql
// icindeki trip_events tablosuyla birebir hizalidir.
package trap

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// TypeCredentialCanary, v0.1'deki tek tuzak turudur (honeytoken).
// nolint gerekce: gosec G101 yanlis pozitifi — sabit adi "credential"
// iceriyor ama deger bir tuzak turu tanimlayicisi, gizli anahtar degil.
const TypeCredentialCanary = "credential_canary" //nolint:gosec

// SubjectTripEvents, sensorlerin TripEvent yayinladigi NATS subject'i.
const SubjectTripEvents = "trip.events.v1"

// ErrNotATrip, gozlemin bir tuzak tetiklemesi olmadigini belirtir.
// Decode bu hatayi dondurdugunde hicbir event yayinlanmaz — sifir
// false-positive tezinin kod duzeyindeki karsiligi (APP-5).
var ErrNotATrip = errors.New("trap: gozlem bir tuzak tetiklemesi degil")

// TripEvent, bir tuzagin tetiklendigini bildiren wire event'idir.
// trip.events.v1 subject'ine JSON olarak yayinlanir.
type TripEvent struct {
	EventID    string          `json:"event_id"`
	TrapID     string          `json:"trap_id"`
	Sensor     string          `json:"sensor"`
	Source     string          `json:"source"`
	ObservedAt time.Time       `json:"observed_at"`
	Raw        json.RawMessage `json:"raw,omitempty"`
}

// Trap, provision edilmis bir tuzagin kaydidir (traps tablosuyla hizali).
// SecretHash API cevaplarina asla cikmaz; duz metin secret yalnizca
// provision aninda Artifacts icinde bir kez doner.
type Trap struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Username   string          `json:"username"`
	SecretHash string          `json:"-"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	CreatedBy  string          `json:"created_by,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	RevokedAt  *time.Time      `json:"revoked_at,omitempty"`
}

// Artifacts, hedefe ekilecek malzemedir; provision cevabinda bir kez doner.
// Secret DB'ye yazilmaz, yalnizca hash'i saklanir (HMAC_KEY ile).
type Artifacts struct {
	Username string `json:"username"`
	Secret   string `json:"secret"`
}

// RawObservation, sensorun decode oncesi ham gozlemidir (ornegin bir
// auth.log satiri). Wire'a cikmaz; sensor icinde Decode'a girdi olur.
type RawObservation struct {
	Sensor     string
	Source     string
	Line       string
	ObservedAt time.Time
}

// Provider, bir tuzak turunun yasam dongusunu yonetir (APP-2).
// Donen username varsayilan olarak `svc_` on ekiyle baslar; seeding yolunda
// profil tabanli inandirici adlarla degistirilebilir (APP-12, bkz.
// WithUsernameGenerator ve internal/seed).
type Provider interface {
	Provision(ctx context.Context, createdBy string) (*Trap, *Artifacts, error)
}

// Decoder, ham gozlemi TripEvent'e cevirir. Gozlem bir tuzak tetiklemesi
// degilse ErrNotATrip doner ve hicbir sey yayinlanmaz (APP-5, sifir-FP).
type Decoder interface {
	Decode(obs RawObservation) (*TripEvent, error)
}
