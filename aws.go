package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type s3Store struct {
	client *s3.Client
}

func (s *s3Store) upload(ctx context.Context, j Job, body io.ReadSeeker) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(j.Bucket), Key: aws.String(j.Key), Body: body,
		ContentLength: aws.Int64(j.Size), ContentType: aws.String(j.ContentType),
	})
	if err != nil {
		return fmt.Errorf("put S3 object: %w", err)
	}
	return nil
}

type sqsSender struct {
	client   *sqs.Client
	queueURL string
}

func (s *sqsSender) send(ctx context.Context, id string, payload []byte) error {
	input := &sqs.SendMessageInput{QueueUrl: aws.String(s.queueURL), MessageBody: aws.String(string(payload))}
	if strings.HasSuffix(s.queueURL, ".fifo") {
		input.MessageGroupId = aws.String("jobs")
		input.MessageDeduplicationId = aws.String(id)
	}
	if _, err := s.client.SendMessage(ctx, input); err != nil {
		return fmt.Errorf("send SQS message: %w", err)
	}
	return nil
}
