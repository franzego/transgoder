package config

import "testing"

func TestLoad_FromEnvironment(t *testing.T) {
	t.Setenv("SERVER_HOST", "127.0.0.1")
	t.Setenv("SERVER_PORT", "9099")
	t.Setenv("WORKER_COUNT", "7")
	t.Setenv("REDIS_HOST", "redis.local")
	t.Setenv("REDIS_PORT", "6380")
	t.Setenv("MINIO_USE_SSL", "true")
	t.Setenv("JWT_TTL_MINUTES", "15")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" || cfg.Server.Port != 9099 {
		t.Fatalf("unexpected server config: %+v", cfg.Server)
	}
	if cfg.Worker.Count != 7 {
		t.Fatalf("expected worker count 7, got %d", cfg.Worker.Count)
	}
	if cfg.Redis.Host != "redis.local" || cfg.Redis.Port != 6380 {
		t.Fatalf("unexpected redis config: %+v", cfg.Redis)
	}
	if !cfg.Minio.UseSSL {
		t.Fatal("expected minio ssl to be true")
	}
	if cfg.JWT.TTLMinutes != 15 {
		t.Fatalf("expected jwt ttl 15, got %d", cfg.JWT.TTLMinutes)
	}
}

func TestHelpersAndAddresses(t *testing.T) {
	if got := getEnv("SOME_MISSING_ENV", "fallback"); got != "fallback" {
		t.Fatalf("expected fallback, got %q", got)
	}

	t.Setenv("BAD_INT", "not-an-int")
	if got := getEnvInt("BAD_INT", 42); got != 42 {
		t.Fatalf("expected int fallback 42, got %d", got)
	}

	t.Setenv("BAD_BOOL", "not-a-bool")
	if got := getEnvBool("BAD_BOOL", true); !got {
		t.Fatalf("expected bool fallback true, got %v", got)
	}

	cfg := &Config{
		Server:   GrpcServerConfig{Host: "0.0.0.0", Port: 8099},
		Redis:    RedisConfig{Host: "localhost", Port: 6379},
		Postgres: PostgresConfig{Host: "db", Port: 5432, User: "u", Password: "p", DB: "d", SSLMode: "disable"},
	}
	if got := cfg.ServerAddr(); got != "0.0.0.0:8099" {
		t.Fatalf("unexpected server addr: %q", got)
	}
	if got := cfg.RedisAddr(); got != "localhost:6379" {
		t.Fatalf("unexpected redis addr: %q", got)
	}
	wantDSN := "host=db port=5432 user=u password=p dbname=d sslmode=disable"
	if got := cfg.PostgresDSN(); got != wantDSN {
		t.Fatalf("unexpected dsn: %q", got)
	}
}
