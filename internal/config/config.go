package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env     string
	Version string

	HTTP     HTTPConfig
	GRPC     GRPCConfig
	Database DatabaseConfig
	Outbox   OutboxConfig
	Log      LogConfig
}

type HTTPConfig struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

type GRPCConfig struct {
	Addr string

	MaxConnectionIdle time.Duration
}

type DatabaseConfig struct {
	DSN             string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	ConnectTimeout  time.Duration

	AutoMigrate bool
}

type OutboxConfig struct {
	PollInterval time.Duration
	BatchSize    int
}

type LogConfig struct {
	Level  string
	Format string
}

func Load() (Config, error) {
	cfg := Config{
		Env:     env("APP_ENV", "development"),
		Version: env("APP_VERSION", "dev"),
		HTTP: HTTPConfig{
			Addr:         env("HTTP_ADDR", ":8080"),
			ReadTimeout:  duration("HTTP_READ_TIMEOUT", 15*time.Second),
			WriteTimeout: duration("HTTP_WRITE_TIMEOUT", 30*time.Second),
			IdleTimeout:  duration("HTTP_IDLE_TIMEOUT", 60*time.Second),

			ShutdownTimeout: duration("SHUTDOWN_TIMEOUT", 15*time.Second),
		},
		GRPC: GRPCConfig{
			Addr: env("GRPC_ADDR", ":9090"),

			MaxConnectionIdle: duration("GRPC_MAX_CONNECTION_IDLE", 0),
		},
		Database: DatabaseConfig{
			DSN:             os.Getenv("DATABASE_DSN"),
			MaxConns:        int32(number("DATABASE_MAX_CONNS", 10)), //nolint:gosec
			MinConns:        int32(number("DATABASE_MIN_CONNS", 2)),  //nolint:gosec
			MaxConnLifetime: duration("DATABASE_CONN_LIFETIME", time.Hour),
			ConnectTimeout:  duration("DATABASE_CONNECT_TIMEOUT", 5*time.Second),
			AutoMigrate:     boolean("DATABASE_AUTO_MIGRATE", true),
		},
		Outbox: OutboxConfig{
			PollInterval: duration("OUTBOX_POLL_INTERVAL", time.Second),
			BatchSize:    number("OUTBOX_BATCH_SIZE", 100),
		},
		Log: LogConfig{
			Level:  env("LOG_LEVEL", "info"),
			Format: env("LOG_FORMAT", "json"),
		},
	}

	if strings.TrimSpace(cfg.Database.DSN) == "" {
		return Config{}, fmt.Errorf("DATABASE_DSN is required")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func number(key string, fallback int) int {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {

		return fallback
	}
	return value
}

func duration(key string, fallback time.Duration) time.Duration {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}

func boolean(key string, fallback bool) bool {
	raw, ok := os.LookupEnv(key)
	if !ok || raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}
