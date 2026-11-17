package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/antonybholmes/go-sys/log"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type job struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Bucket      string    `json:"bucket"`
	Key         string    `json:"key"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
}

type objectStore interface {
	upload(ctx context.Context, job job, body io.ReadSeeker) error
}

type jobStore interface {
	createJob(ctx context.Context, job job) error
}

type submitHandler struct {
	bucket  string
	maxBody int64
	objects objectStore
	jobs    jobStore
}

func (h *submitHandler) submit(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxBody)

	file, fileHeader, err := c.Request.FormFile("file")

	// clean up but seems unnecessary if no multipart form was created
	if c.Request.MultipartForm != nil {
		defer func() {
			if err := c.Request.MultipartForm.RemoveAll(); err != nil {
				log.Error().Msgf("remove multipart temporary files: %v", err)
			}
		}()
	}

	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "a multipart file field named file is required"})
		}
		return
	}
	defer file.Close()

	id, err := uuid.NewV7()
	if err != nil {
		log.Error().Msgf("generate job ID: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create job"})
		return
	}
	contentType := fileHeader.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !validMetadata(fileHeader.Filename, 1024) || !validMetadata(contentType, 255) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file metadata"})
		return
	}

	j := job{
		ID:          id.String(),
		Type:        "crispr", // or "crispr" depending on the job type
		Bucket:      h.bucket,
		Key:         "jobs/" + id.String() + "/upload",
		Filename:    fileHeader.Filename,
		ContentType: contentType,
		Size:        fileHeader.Size,
		CreatedAt:   time.Now().UTC(),
	}

	if err := h.objects.upload(c.Request.Context(), j, file); err != nil {
		log.Error().Msgf("upload job file: job_id=%s error=%v", j.ID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "file upload failed"})
		return
	}

	if err := h.jobs.createJob(c.Request.Context(), j); err != nil {
		// A failed commit can have an unknown outcome, so deleting the object here
		// could destroy a successfully committed job's input.
		log.Error().Msgf("persist uploaded job; object may need reconciliation: job_id=%s bucket=%s key=%s error=%v", j.ID, j.Bucket, j.Key, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not persist job"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"job_id": j.ID})
}

func validMetadata(value string, maxBytes int) bool {
	return len(value) <= maxBytes && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
