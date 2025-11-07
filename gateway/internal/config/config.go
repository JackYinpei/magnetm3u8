package config

import (
	"os"
	"strconv"
	"time"
)

// Config captures runtime options for the gateway service.
type Config struct {
	Port              string
	DBPath            string
	SessionCookieName string
	SessionTTL        time.Duration
	StaticDir         string
	AdminUsername     string
	AdminPassword     string
}

// Load assembles configuration from flags and environment variables.
func Load(portFlag string) Config {
	cfg := Config{
		Port:              pickFirst(os.Getenv("GATEWAY_PORT"), portFlag, "8080"),
		DBPath:            pickFirst(os.Getenv("GATEWAY_DB_PATH"), "gateway.db"),
		SessionCookieName: pickFirst(os.Getenv("SESSION_COOKIE_NAME"), "gateway_session"),
		StaticDir:         pickFirst(os.Getenv("STATIC_DIR"), "./static"),
		AdminUsername:     pickFirst(os.Getenv("DEFAULT_ADMIN_USERNAME"), "admin"),
		AdminPassword:     pickFirst(os.Getenv("DEFAULT_ADMIN_PASSWORD"), "ChangeMe!123"),
	}

	cfg.SessionTTL = parseDurationHours(pickFirst(os.Getenv("SESSION_TTL_HOURS"), "168")) // one week

	return cfg
}

func pickFirst(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func parseDurationHours(raw string) time.Duration {
	hours, err := strconv.Atoi(raw)
	if err != nil || hours <= 0 {
		hours = 168
	}
	return time.Duration(hours) * time.Hour
}
