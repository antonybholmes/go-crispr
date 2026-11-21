package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/antonybholmes/go-sys/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type (
	PostgresStore struct {
		pool *pgxpool.Pool
	}

	MessageSender interface {
		Send(ctx context.Context, payload []byte) error
	}

	OutboxPublisher interface {
		PublishOne(context.Context, MessageSender) (bool, error)
	}
)

var schema string

func (s *PostgresStore) CreateJob(ctx context.Context, j jobs.Job) error {
	payload, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("marshal job: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin job transaction: %w", err)
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, `INSERT INTO jobs
		(id, type, bucket, object_key, filename, content_type, size_bytes, created_at)
		VALUES (:id, :type, :bucket, :object_key, :filename, :content_type, :size_bytes, :created_at)`,
		map[string]any{
			"id":           j.ID,
			"bucket":       j.Bucket,
			"object_key":   j.Key,
			"filename":     j.Filename,
			"content_type": j.ContentType,
			"size_bytes":   j.Size,
			"created_at":   j.CreatedAt,
		})
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO job_outbox (job_id, payload, created_at)
		VALUES ($1, $2, $3)`, j.ID, payload, j.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert outbox message: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit job transaction: %w", err)
	}
	return nil
}

func (s *PostgresStore) PublishOne(ctx context.Context, sender MessageSender) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin outbox transaction: %w", err)
	}
	defer tx.Rollback(context.Background())

	var id string
	var payload []byte

	err = tx.QueryRow(ctx, `SELECT 
		job_id::text, 
		payload 
		FROM job_outbox
		WHERE published_at IS NULL ORDER BY created_at, job_id
		FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&id, &payload)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("select outbox message: %w", err)
	}
	if err := sender.Send(ctx, payload); err != nil {
		return false, fmt.Errorf("send job %s: %w", id, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE job_outbox SET published_at = now() WHERE job_id = $1`, id); err != nil {
		return false, fmt.Errorf("mark job %s published: %w", id, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit published job %s: %w", id, err)
	}
	return true, nil
}
