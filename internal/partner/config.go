package partner

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	KitchenGRPCAddr string

	APIKey string

	HTTPAddr string

	LogLevel  string
	LogFormat string

	SyncMenu bool

	StartupTimeout time.Duration

	Policy Policy
}

type Policy struct {
	AutoAccept bool

	AutoRejectOver int64

	PrepMinutes int

	AcceptDelay   time.Duration
	CookingDelay  time.Duration
	ReadyDelay    time.Duration
	DispatchDelay time.Duration
	DeliveryDelay time.Duration

	RestockInterval time.Duration
	RestockTo       int
}

func LoadConfig() (Config, error) {
	cfg := Config{
		KitchenGRPCAddr: env("KITCHEN_GRPC_ADDR", "kitchen:9090"),
		APIKey:          os.Getenv("PARTNER_API_KEY"),
		HTTPAddr:        env("HTTP_ADDR", ":8081"),
		LogLevel:        env("LOG_LEVEL", "info"),
		LogFormat:       env("LOG_FORMAT", "json"),
		SyncMenu:        boolean("PARTNER_SYNC_MENU", true),
		StartupTimeout:  duration("PARTNER_STARTUP_TIMEOUT", 90*time.Second),
		Policy: Policy{
			AutoAccept:      boolean("PARTNER_AUTO_ACCEPT", true),
			AutoRejectOver:  int64(number("PARTNER_AUTO_REJECT_OVER", 0)),
			PrepMinutes:     number("PARTNER_PREP_MINUTES", 25),
			AcceptDelay:     duration("PARTNER_ACCEPT_DELAY", 3*time.Second),
			CookingDelay:    duration("PARTNER_COOKING_DELAY", 5*time.Second),
			ReadyDelay:      duration("PARTNER_READY_DELAY", 10*time.Second),
			DispatchDelay:   duration("PARTNER_DISPATCH_DELAY", 5*time.Second),
			DeliveryDelay:   duration("PARTNER_DELIVERY_DELAY", 15*time.Second),
			RestockInterval: duration("PARTNER_RESTOCK_INTERVAL", 2*time.Minute),
			RestockTo:       number("PARTNER_RESTOCK_TO", 20),
		},
	}

	if strings.TrimSpace(cfg.APIKey) == "" {
		return Config{}, fmt.Errorf("PARTNER_API_KEY is required")
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
