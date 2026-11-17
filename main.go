package main

import (
	"context"
	"errors"
	"fmt"
	appconfig "go-crispr/config"

	"os"
	"time"

	"github.com/antonybholmes/go-sys/log"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/bytedance/gopkg/util/logger"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func run(cfg *appconfig.AppConfig) error {

	ctx := context.Background()
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return fmt.Errorf("load AWS configuration: %w", err)
	}

	if awsCfg.Region == "" {
		return errors.New("AWS region is required; set AWS_REGION")
	}

	pool, err := pgxpool.New(ctx, cfg.App.DatabaseURL)
	if err != nil {
		return fmt.Errorf("configure PostgreSQL: %w", err)
	}

	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}

	if _, err := pool.Exec(ctx, schema); err != nil {
		return fmt.Errorf("initialize schema: %w", err)
	}

	store := &postgresStore{pool: pool}
	sender := &sqsSender{client: sqs.NewFromConfig(awsCfg), queueURL: cfg.App.QueueURL}
	handler := &submitHandler{
		bucket: cfg.App.Bucket, maxBody: cfg.App.MaxBodyBytes, objects: &s3Store{client: s3.NewFromConfig(awsCfg)},
		jobs: store,
	}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.MaxMultipartMemory = 8 << 20
	router.POST("/submit", handler.submit)

	// The outbox runs alongside HTTP so it can publish accepted jobs to SQS.
	go runOutbox(ctx, store, sender, time.Duration(cfg.App.PollIntervalSec)*time.Second)

	logger.Info("server starting", "address", cfg.App.Port)

	if err := router.Run(cfg.App.Port); err != nil {
		return fmt.Errorf("serve HTTP: %w", err)
	}
	return nil
}

type outboxPublisher interface {
	publishOne(context.Context, messageSender) (bool, error)
}

// runOutbox repeatedly delivers pending messages, polling after an empty queue
// or an error until its context is canceled.
func runOutbox(ctx context.Context, store outboxPublisher, sender messageSender, interval time.Duration) {
	for ctx.Err() == nil {
		sent, err := store.publishOne(ctx, sender)
		if err != nil && ctx.Err() == nil {
			log.Error().Msgf("publish outbox message: %v", err)
		}
		if sent && err == nil {
			continue
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func init() {
	log.SetAppName(appconfig.AppName)
}

func main() {
	cfg := appconfig.Instance()

	err := run(cfg)

	if err != nil {
		logger.Error("application stopped", "error", err)
		os.Exit(1)
	}
}
