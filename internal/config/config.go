package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Business BusinessConfig `yaml:"business"`
}

type ServerConfig struct {
	Port string `yaml:"port"`
}

type DatabaseConfig struct {
	URL string `yaml:"url"`
}

type AuthConfig struct {
	CustomerTokenPrefix string `yaml:"customer_token_prefix"`
	OrderServiceToken   string `yaml:"order_service_token"`
}

type BusinessConfig struct {
	MinimumBalanceReserve float64 `yaml:"minimum_balance_reserve"`
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Database.URL == "" {
		return nil, fmt.Errorf("database.url is required")
	}
	if cfg.Auth.OrderServiceToken == "" {
		return nil, fmt.Errorf("auth.order_service_token is required")
	}
	if cfg.Server.Port == "" {
		cfg.Server.Port = "8080"
	}
	if cfg.Auth.CustomerTokenPrefix == "" {
		cfg.Auth.CustomerTokenPrefix = "customer:"
	}
	if cfg.Business.MinimumBalanceReserve == 0 {
		cfg.Business.MinimumBalanceReserve = 100.0
	}
	return &cfg, nil
}
