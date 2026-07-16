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
	t.Setenv("HMAC_KEY", "secret")
	t.Setenv("HTTP_ADDR", ":9090")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if cfg.httpAddr != ":9090" {
		t.Errorf("httpAddr = %q, istenen :9090", cfg.httpAddr)
	}
	if cfg.dbDSN != "postgres://x" || cfg.natsURL != "nats://x" || cfg.hmacKey != "secret" {
		t.Errorf("config degerleri env ile eslesmiyor: %+v", cfg)
	}
}

func TestLoadConfig_DefaultHTTPAddr(t *testing.T) {
	t.Setenv("DB_DSN", "postgres://x")
	t.Setenv("NATS_URL", "nats://x")
	t.Setenv("HMAC_KEY", "secret")
	t.Setenv("HTTP_ADDR", "")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("beklenmeyen hata: %v", err)
	}
	if cfg.httpAddr != ":8080" {
		t.Errorf("varsayilan httpAddr = %q, istenen :8080", cfg.httpAddr)
	}
}
