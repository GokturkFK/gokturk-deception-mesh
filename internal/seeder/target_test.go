package seeder

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeTarget, Target arayuzunu gercek SSH olmadan test etmek icin taklit
// eder. Cagrilan komutlari sirayla kaydeder; belirli bir komut on ekine
// hata donmesi istenirse failOn ile ayarlanir.
type fakeTarget struct {
	calls   []string
	outputs map[string]string
	failOn  string
	failErr error
}

func (f *fakeTarget) Run(_ context.Context, cmd string) (string, error) {
	f.calls = append(f.calls, cmd)
	if f.failOn != "" && strings.HasPrefix(cmd, f.failOn) {
		return "", f.failErr
	}
	if out, ok := f.outputs[cmd]; ok {
		return out, nil
	}
	return "", nil
}

func TestReadExisting_ParsesUsernamesAndUIDs(t *testing.T) {
	ft := &fakeTarget{outputs: map[string]string{
		readPasswdCmd: "root:x:0:0:root:/root:/bin/bash\n" +
			"backup:x:142:142:backup service account,,,:/var/lib/backup:/usr/sbin/nologin\n" +
			"\n# yorum satiri\n",
	}}

	ex, err := ReadExisting(context.Background(), ft)
	if err != nil {
		t.Fatalf("ReadExisting: %v", err)
	}
	if !ex.Usernames["root"] || !ex.Usernames["backup"] {
		t.Errorf("beklenen kullanicilar eksik: %+v", ex.Usernames)
	}
	if !ex.UIDs[0] || !ex.UIDs[142] {
		t.Errorf("beklenen UID'ler eksik: %+v", ex.UIDs)
	}
	if len(ex.Usernames) != 2 {
		t.Errorf("bos/yorum satirlari da sayilmis: %d kullanici", len(ex.Usernames))
	}
}

func TestReadExisting_RunErrorPropagates(t *testing.T) {
	ft := &fakeTarget{failOn: "cat", failErr: errors.New("ssh koptu")}

	if _, err := ReadExisting(context.Background(), ft); err == nil {
		t.Fatal("Run hatasi ReadExisting'ten donmeliydi")
	}
}

func TestApply_RunsStepsInOrder(t *testing.T) {
	ft := &fakeTarget{}
	id := testIdentity()

	if err := Apply(context.Background(), ft, id, "s3cret"); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(ft.calls) != 4 {
		t.Fatalf("4 komut beklenirdi, gelen: %d (%v)", len(ft.calls), ft.calls)
	}
	if !strings.HasPrefix(ft.calls[0], "groupadd") {
		t.Errorf("1. komut groupadd olmali: %q", ft.calls[0])
	}
	if !strings.HasPrefix(ft.calls[1], "useradd") {
		t.Errorf("2. komut useradd olmali: %q", ft.calls[1])
	}
	if !strings.Contains(ft.calls[2], "chpasswd") {
		t.Errorf("3. komut chpasswd olmali: %q", ft.calls[2])
	}
	if !strings.HasPrefix(ft.calls[3], "touch") {
		t.Errorf("4. komut touch (backdate) olmali: %q", ft.calls[3])
	}
}

func TestApply_GroupaddFailureStopsBeforeUseradd(t *testing.T) {
	ft := &fakeTarget{failOn: "groupadd", failErr: errors.New("gid catisiyor")}

	err := Apply(context.Background(), ft, testIdentity(), "s3cret")
	if err == nil {
		t.Fatal("groupadd hatasi Apply'dan donmeliydi")
	}
	if len(ft.calls) != 1 {
		t.Errorf("groupadd basarisizken useradd denenmemeli, cagrilar: %v", ft.calls)
	}
}

func TestApply_UseraddFailureStopsBeforePassword(t *testing.T) {
	ft := &fakeTarget{failOn: "useradd", failErr: errors.New("uid catisiyor")}

	err := Apply(context.Background(), ft, testIdentity(), "s3cret")
	if err == nil {
		t.Fatal("useradd hatasi Apply'dan donmeliydi")
	}
	if len(ft.calls) != 2 {
		t.Errorf("useradd basarisizken parola/backdate denenmemeli, cagrilar: %v", ft.calls)
	}
}

func TestApply_ChpasswdFailureStopsBeforeBackdate(t *testing.T) {
	ft := &fakeTarget{failOn: "printf", failErr: errors.New("chpasswd yok")}

	err := Apply(context.Background(), ft, testIdentity(), "s3cret")
	if err == nil {
		t.Fatal("chpasswd hatasi Apply'dan donmeliydi")
	}
	if len(ft.calls) != 3 {
		t.Errorf("parola basarisizken backdate denenmemeli, cagrilar: %v", ft.calls)
	}
}

func TestRevoke_RunsUserdel(t *testing.T) {
	ft := &fakeTarget{}

	if err := Revoke(context.Background(), ft, "backup"); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if len(ft.calls) != 1 || !strings.HasPrefix(ft.calls[0], "userdel") {
		t.Errorf("tek komut userdel olmali: %v", ft.calls)
	}
}

func TestRevoke_UserdelErrorPropagates(t *testing.T) {
	ft := &fakeTarget{failOn: "userdel", failErr: errors.New("hesap mesgul")}

	if err := Revoke(context.Background(), ft, "backup"); err == nil {
		t.Fatal("userdel hatasi Revoke'tan donmeliydi")
	}
}
