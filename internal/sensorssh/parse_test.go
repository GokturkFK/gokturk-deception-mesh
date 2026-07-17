package sensorssh

import "testing"

func TestParseAuthLine(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantOK   bool
		user     string
		source   string
		accepted bool
	}{
		{
			name:     "accepted password",
			line:     "May 10 12:00:00 host sshd[123]: Accepted password for svc_backup from 10.0.0.99 port 54321 ssh2",
			wantOK:   true,
			user:     "svc_backup",
			source:   "10.0.0.99",
			accepted: true,
		},
		{
			name:     "failed password",
			line:     "May 10 12:00:01 host sshd[123]: Failed password for svc_backup from 10.0.0.99 port 54321 ssh2",
			wantOK:   true,
			user:     "svc_backup",
			source:   "10.0.0.99",
			accepted: false,
		},
		{
			name:     "failed invalid user",
			line:     "Jul 17 09:15:22 srv sshd[9001]: Failed password for invalid user admin from 203.0.113.7 port 40012 ssh2",
			wantOK:   true,
			user:     "admin",
			source:   "203.0.113.7",
			accepted: false,
		},
		{
			name:   "accepted publickey ignore",
			line:   "May 10 12:00:00 host sshd[123]: Accepted publickey for root from 10.0.0.1 port 22 ssh2",
			wantOK: false,
		},
		{
			name:   "non-auth sshd line",
			line:   "May 10 12:00:00 host sshd[123]: Connection closed by 10.0.0.5 port 22",
			wantOK: false,
		},
		{
			name:   "garbage",
			line:   "bu bir log satiri degil",
			wantOK: false,
		},
		{
			name:   "empty",
			line:   "",
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, ok := ParseAuthLine(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, istenen %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if ev.Username != tc.user {
				t.Errorf("username = %q, istenen %q", ev.Username, tc.user)
			}
			if ev.Source != tc.source {
				t.Errorf("source = %q, istenen %q", ev.Source, tc.source)
			}
			if ev.Accepted != tc.accepted {
				t.Errorf("accepted = %v, istenen %v", ev.Accepted, tc.accepted)
			}
		})
	}
}
