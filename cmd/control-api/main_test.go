package main

import "testing"

func TestLoadConfig_MissingRequired(t *testing.T) {
	t.Setenv("DB_DSN", "")
	t.Setenv("NATS_URL", "")
	t.Setenv("HMAC_KEY", "")

	if _, err := loadConfig(); err == nil {
		t.Fatal("beklenen hata alinmadi: zorunlu env degiskenleri eksikken loadConfig basarili oldu")
	}
}

func TestLoadConfig_AllPresent(t *testing.T) {
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("NATS_URL", "nats://x")
	t.Setenv("HMAC_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("HTTP_ADDR", ":9090")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if cfg.httpAddr != ":9090" {
		t.Errorf("httpAddr = %q, istenen :9090", cfg.httpAddr)
	}
	if cfg.dbDSN != "postgres://x" || cfg.natsURL != "nats://x" || cfg.hmacKey != "0123456789abcdef0123456789abcdef" {
		t.Errorf("config degerleri env ile eslesmiyor: %+v", cfg)
	}
}

func TestLoadConfig_DefaultHTTPAddr(t *testing.T) {
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("NATS_URL", "nats://x")
	t.Setenv("HMAC_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("HTTP_ADDR", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if cfg.httpAddr != ":8080" {
		t.Errorf("varsayilan httpAddr = %q, istenen :8080", cfg.httpAddr)
	}
}

func TestLoadConfig_ShortHMACKey(t *testing.T) {
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("NATS_URL", "nats://x")
	t.Setenv("HMAC_KEY", "cok-kisa")

	if _, err := loadConfig(); err == nil {
		t.Fatal("32 bayttan kisa HMAC_KEY reddedilmeliydi")
	}
}

func TestHealthURL(t *testing.T) {
	cases := map[string]string{
		":8080":         "http://127.0.0.1:8080/healthz",
		"0.0.0.0:9090":  "http://127.0.0.1:9090/healthz",
		"10.0.0.5:8081": "http://127.0.0.1:8081/healthz",
	}
	for addr, want := range cases {
		if got := healthURL(addr); got != want {
			t.Errorf("healthURL(%q) = %q, istenen %q", addr, got, want)
		}
	}
}

func TestRunHealthcheck_NoListener(t *testing.T) {
	// Dinleyen servis yokken healthcheck 1 donmeli.
	if code := runHealthcheck(":1"); code != 1 {
		t.Errorf("runHealthcheck(:1) = %d, istenen 1", code)
	}
}
