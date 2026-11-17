package config

import (
	"os"
	"sync"

	"github.com/antonybholmes/go-sys/log"

	"go.yaml.in/yaml/v4"
)

type AppConfig struct {
	App struct {
		URL             string   `yaml:"url"`
		Domain          string   `yaml:"domain"`
		LogFile         string   `yaml:"logFile"`
		Port            string   `yaml:"port"`
		TrustedProxies  []string `yaml:"trustedProxies"`
		DatabaseURL     string   `yaml:"databaseUrl"`
		Bucket          string   `yaml:"bucket"`
		QueueURL        string   `yaml:"queueUrl"`
		MaxBodyBytes    int64    `yaml:"maxBodyBytes"`
		PollIntervalSec int      `yaml:"pollIntervalSeconds"`
	} `yaml:"app"`
}

const (
	Name       = "CRISPR Job Server"
	AppName    = "crispr-job-server"
	Copyright  = "Copyright (C) 2026 Antony Holmes"
	ConfigFile = "config/app.yaml"
)

var (
	cfg  *AppConfig
	once sync.Once
)

func LoadConfig() *AppConfig {
	data, err := os.ReadFile(ConfigFile)

	if err != nil {
		log.Fatal().Msgf("failed to parse config file: %v", err)
		os.Exit(1)
	}

	var cfg AppConfig

	if err := yaml.Load(data, &cfg); err != nil {
		log.Fatal().Msgf("failed to parse config file: %v", err)
		os.Exit(1)
	}

	return &cfg
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func Instance() *AppConfig {
	once.Do(func() {
		cfg = LoadConfig()
	})

	return cfg
}
