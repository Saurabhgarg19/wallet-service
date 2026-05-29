package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL         string
	Port                string
	CustomerTokenPrefix string
	OrderServiceToken   string
}

func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	orderToken := os.Getenv("ORDER_SERVICE_TOKEN")
	if orderToken == "" {
		return nil, fmt.Errorf("ORDER_SERVICE_TOKEN is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	prefix := os.Getenv("CUSTOMER_TOKEN_PREFIX")
	if prefix == "" {
		prefix = "customer:"
	}

	return &Config{
		DatabaseURL:         dbURL,
		Port:                port,
		CustomerTokenPrefix: prefix,
		OrderServiceToken:   orderToken,
	}, nil
}
