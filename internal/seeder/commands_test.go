package seeder

import (
	"strings"
	"testing"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/seed"
)

func testIdentity() seed.Identity {
	return seed.Identity{
		Profile:  "debian-service",
		Username: "backup",
		UID:      142,
		GID:      142,
		Shell:    "/usr/sbin/nologin",
		Home:     "/var/lib/backup",
		GECOS:    "backup service account,,,",
		SeededAt: time.Date(2026, 3, 1, 10, 30, 0, 0, time.UTC),
	}
}

func TestShellQuote_PlainString(t *testing.T) {
	if got := shellQuote("backup"); got != "'backup'" {
		t.Errorf("shellQuote = %q", got)
	}
}

func TestShellQuote_EscapesSingleQuote(t *testing.T) {
	got := shellQuote("o'brien")
	want := `'o'\''brien'`
	if got != want {
		t.Errorf("shellQuote = %q, istenen %q", got, want)
	}
}

func TestShellQuote_NeutralizesShellMetacharacters(t *testing.T) {
	// Kacisleme sonrasi payload tek-tirnaklarin ICINDE, yorumlanmadan durmali.
	payload := "; rm -rf / #"
	got := shellQuote(payload)
	if got != "'"+payload+"'" {
		t.Errorf("shellQuote metakarakterleri notrlemedi: %q", got)
	}
}

func TestGroupaddCmd_ContainsGIDAndUsername(t *testing.T) {
	cmd := groupaddCmd(testIdentity())
	want := "groupadd --gid '142' 'backup'"
	if cmd != want {
		t.Errorf("groupaddCmd = %q, istenen %q", cmd, want)
	}
}

func TestUseraddCmd_ContainsExpectedFields(t *testing.T) {
	cmd := useraddCmd(testIdentity())

	for _, want := range []string{
		"useradd", "--uid '142'", "--gid '142'",
		"--home-dir '/var/lib/backup'", "--create-home",
		"--shell '/usr/sbin/nologin'",
		"--comment 'backup service account,,,'",
		"'backup'",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("useraddCmd = %q, %q icermeli", cmd, want)
		}
	}
}

func TestChpasswdCmd_QuotesUsernameAndSecret(t *testing.T) {
	cmd := chpasswdCmd("backup", "sup3r-secret_val")
	if !strings.Contains(cmd, "'backup'") || !strings.Contains(cmd, "'sup3r-secret_val'") {
		t.Errorf("chpasswdCmd = %q, kullanici adi ve secret kacislenmis olmali", cmd)
	}
	if !strings.HasSuffix(cmd, "| chpasswd") {
		t.Errorf("chpasswdCmd chpasswd'ye piplenmiyor: %q", cmd)
	}
}

func TestBackdateCmd_FormatsTimestampAndHome(t *testing.T) {
	cmd := backdateCmd(testIdentity())
	want := "touch -t 202603011030.00 '/var/lib/backup'"
	if cmd != want {
		t.Errorf("backdateCmd = %q, istenen %q", cmd, want)
	}
}

func TestUserdelCmd_RemovesHome(t *testing.T) {
	cmd := userdelCmd("backup")
	if !strings.Contains(cmd, "--remove") || !strings.Contains(cmd, "'backup'") {
		t.Errorf("userdelCmd = %q", cmd)
	}
}
