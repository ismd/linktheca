package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	HTTPAddr  string `env:"LINKTHECA_HTTP_ADDR" envDefault:":8080"`
	LogLevel  string `env:"LINKTHECA_LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"LINKTHECA_LOG_FORMAT" envDefault:"text"`

	DBDSN string `env:"LINKTHECA_DB_DSN,required"`

	JWTSecret     string        `env:"LINKTHECA_JWT_SECRET,required"`
	JWTAccessTTL  time.Duration `env:"LINKTHECA_JWT_ACCESS_TTL" envDefault:"15m"`
	JWTRefreshTTL time.Duration `env:"LINKTHECA_JWT_REFRESH_TTL" envDefault:"720h"`

	RegistrationEnabled bool `env:"LINKTHECA_REGISTRATION_ENABLED" envDefault:"true"`

	CORSOrigins []string `env:"LINKTHECA_CORS_ORIGINS" envSeparator:","`
}

func Load() (*Config, error) {
	var cfg Config

	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	if len(cfg.JWTSecret) < 32 {
		return nil, errors.New("LINKTHECA_JWT_SECRET must be at least 32 bytes")
	}

	return &cfg, nil
}
