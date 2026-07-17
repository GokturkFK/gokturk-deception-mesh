// Package sensorssh, SSH auth log'unu takip edip parse eden sensor mantigini icerir.
// APP-4: ham satirdan kullanici + kaynak IP cikarimi ve rotate/truncate'e dayanikli
// dosya takibi. APP-5'te bu cikti Decode + NATS publish'e beslenecek.
package sensorssh

import "regexp"

// authLineRe, OpenSSH'in parola denemesi satirlarini yakalar:
//
//	... sshd[123]: Accepted password for svc_backup from 10.0.0.99 port 54321 ssh2
//	... sshd[123]: Failed password for invalid user admin from 1.2.3.4 port 22 ssh2
//
// Yalnizca "password" denemeleri ilgilendiriyor: canary credential'i bir parola
// oldugundan tuzak ancak parola ile tetiklenir (publickey degil).
var authLineRe = regexp.MustCompile(`(Accepted|Failed) password for (?:invalid user )?(\S+) from (\S+) port \d+`)

// AuthEvent, bir SSH parola denemesinden cikarilan alanlardir.
type AuthEvent struct {
	Username string
	Source   string // kaynak IP (veya hostname)
	Accepted bool   // true: Accepted, false: Failed
}

// ParseAuthLine, tek bir log satirini parse eder. Satir bir SSH parola denemesi
// degilse ok=false doner (hicbir sey uretilmez — sifir-FP'nin ilk kapisi).
func ParseAuthLine(line string) (AuthEvent, bool) {
	m := authLineRe.FindStringSubmatch(line)
	if m == nil {
		return AuthEvent{}, false
	}
	return AuthEvent{
		Username: m[2],
		Source:   m[3],
		Accepted: m[1] == "Accepted",
	}, true
}
