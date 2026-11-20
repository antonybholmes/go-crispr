package config

import (
	"crypto/ecdsa"
	"os"
	"sync"

	"github.com/antonybholmes/go-sys/log"
	"github.com/golang-jwt/jwt/v5"

	"go.yaml.in/yaml/v4"
)

type AppConfig struct {
	Auth struct {
		Keys struct {
			JwtES256PrivateKey *ecdsa.PrivateKey `yaml:"-"`
			JwtES256PublicKey  *ecdsa.PublicKey  `yaml:"-"`
		} `yaml:"-"`
	} `yaml:"-"`
	Aws struct {
		Folder string `yaml:"folder"`
		Bucket string `yaml:"-"`
		Sqs    struct {
			QueueURL string `yaml:"-"`
		} `yaml:"-"`
	} `yaml:"-"`
	App struct {
		URL             string   `yaml:"url"`
		Domain          string   `yaml:"domain"`
		LogFile         string   `yaml:"logFile"`
		Port            string   `yaml:"port"`
		DatabaseURL     string   `yaml:"-"`
		TrustedProxies  []string `yaml:"trustedProxies"`
		MaxBodyBytes    int64    `yaml:"maxBodyBytes"`
		PollIntervalSec int      `yaml:"pollIntervalSeconds"`
	} `yaml:"app"`
	Cors struct {
		AllowOrigins     []string `yaml:"allowOrigins"`
		AllowMethods     []string `yaml:"allowMethods"`
		AllowHeaders     []string `yaml:"allowHeaders"`
		AllowCredentials bool     `yaml:"allowCredentials"`
		MaxAge           int      `yaml:"maxAge"`
	} `yaml:"cors"`
}

const (
	Name       = "EDB Job Server"
	AppName    = "edb-job-server"
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

	cfg.App.DatabaseURL = envOr("DATABASE_URL", cfg.App.DatabaseURL)
	cfg.Aws.Bucket = envOr("AWS_BUCKET", cfg.Aws.Bucket)
	cfg.Aws.Sqs.QueueURL = envOr("AWS_SQS_QUEUE_URL", cfg.Aws.Sqs.QueueURL)

	//
	// Load EC keys
	//

	bytes, err := os.ReadFile("jwt.es256.private.pem")
	if err != nil {
		log.Fatal().Msgf("%s", err)
	}

	cfg.Auth.Keys.JwtES256PrivateKey, err = jwt.ParseECPrivateKeyFromPEM(bytes)
	if err != nil {
		log.Fatal().Msgf("%s", err)
	}

	bytes, err = os.ReadFile("jwt.es256.public.pem")
	if err != nil {
		log.Fatal().Msgf("%s", err)
	}

	cfg.Auth.Keys.JwtES256PublicKey, err = jwt.ParseECPublicKeyFromPEM(bytes)
	if err != nil {
		log.Fatal().Msgf("%s", err)
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
