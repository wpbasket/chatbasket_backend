package db

import (
	"chatbasket/utils"
	"time"
)

type PostgresConfig struct {
	DatabaseURL           string
	MaxConns              int32
	MinConns              int32
	MinIdleConns          int32
	MaxConnLifetime       time.Duration
	MaxConnIdleTime       time.Duration
	HealthCheckPeriod     time.Duration
	MaxConnLifetimeJitter time.Duration
}

func LoadPostgresConfig() (*PostgresConfig, error) {
	dsn, err := utils.LoadKeyFromEnv("DATABASE_URL_PG_DEV")
	if err != nil {
		return nil, err
	}
	return &PostgresConfig{
		DatabaseURL:           dsn,
		MaxConns:              30,
		MinConns:              2,
		MinIdleConns:          2,
		MaxConnLifetime:       30 * time.Minute,
		MaxConnIdleTime:       2 * time.Minute,
		HealthCheckPeriod:     1 * time.Minute,
		MaxConnLifetimeJitter: 5 * time.Minute,
	}, nil
}
