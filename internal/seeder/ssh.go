package seeder

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// dialTimeout, SSH el sikismasi icin ust siniirdir.
const dialTimeout = 10 * time.Second

// SSHTarget, Target arayuzunu gercek bir SSH baglantisi uzerinden karsilar.
type SSHTarget struct {
	client *ssh.Client
}

// DialSSH, verilen adrese ("host:port") private key ile baglanir. Host key
// dogrulamasi known_hosts dosyasina karsi yapilir; InsecureIgnoreHostKey
// KASITLI OLARAK kullanilmaz — bir SSH hedefine dogrulamasiz baglanmak,
// aradaki-adam saldirisina karsi tuzagi seeding eden asil kanali acar
// (bkz. docs/SEEDING.md OPS-11 sorumluluklari: "SSH private key ... duz
// metin bulunmamali" ile ayni ruh: transport kanalinin kendisi guvenilir
// olmali).
func DialSSH(addr, user, privateKeyPath, knownHostsPath string) (*SSHTarget, error) {
	keyBytes, err := os.ReadFile(privateKeyPath) //nolint:gosec // G304: yol operator tarafindan -ssh-key ile verilir, harici/aginan girdi degil.
	if err != nil {
		return nil, fmt.Errorf("seeder: ssh anahtari okunamadi (%s): %w", privateKeyPath, err)
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("seeder: ssh anahtari cozulemedi (%s): %w", privateKeyPath, err)
	}

	hostKeyCallback, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("seeder: known_hosts okunamadi (%s): %w", knownHostsPath, err)
	}

	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCallback,
		Timeout:         dialTimeout,
	}

	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("seeder: %s adresine baglanilamadi: %w", addr, err)
	}
	return &SSHTarget{client: client}, nil
}

// Close, SSH baglantisini kapatir.
func (t *SSHTarget) Close() error {
	return t.client.Close()
}

// Run, Target arayuzunu karsilar: tek bir komutu ayri bir oturumda calistirir.
// ctx iptal edilirse oturuma SIGKILL gonderilip baglanti serbest birakilir.
func (t *SSHTarget) Run(ctx context.Context, cmd string) (string, error) {
	session, err := t.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("seeder: ssh oturumu acilamadi: %w", err)
	}
	defer func() { _ = session.Close() }()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- session.Run(cmd) }()

	select {
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		return "", fmt.Errorf("seeder: komut iptal edildi (%s): %w", cmd, ctx.Err())
	case runErr := <-done:
		if runErr != nil {
			return stdout.String(), fmt.Errorf("seeder: komut basarisiz (%s): %w (stderr: %s)",
				cmd, runErr, stderr.String())
		}
		return stdout.String(), nil
	}
}
