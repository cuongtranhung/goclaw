package minio

import (
	"context"
	"fmt"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

// MediaStore handles automatic upload of agent-generated media (TTS audio,
// generated images) to MinIO, returning presigned URLs that channels
// (e.g. Telegram) can use directly instead of local file paths.
type MediaStore struct {
	client        *Client
	presignExpiry time.Duration
	keyPrefix     string
}

// NewMediaStore creates a MediaStore using the provided client and config.
func NewMediaStore(client *Client, cfg config.MinIOMediaStoreConfig) *MediaStore {
	expiry := time.Duration(cfg.PresignExpiry) * time.Minute
	if expiry <= 0 {
		expiry = 24 * time.Hour // default 24 h
	}
	prefix := cfg.KeyPrefix
	if prefix == "" {
		prefix = "media/"
	}
	return &MediaStore{
		client:        client,
		presignExpiry: expiry,
		keyPrefix:     prefix,
	}
}

// UploadMedia uploads the local temp file at localPath to MinIO and returns
// a presigned GET URL valid for the configured duration.
//
// Key pattern: "{keyPrefix}{YYYY-MM-DD}/{basename}"
// The local file is NOT deleted here — the caller (manager.go) skips the usual
// os.Remove() when the URL is HTTP.
func (m *MediaStore) UploadMedia(ctx context.Context, localPath, contentType string) (string, error) {
	if contentType == "" {
		contentType = inferContentType(localPath)
	}

	// Build object key: prefix + date subfolder + filename
	date := time.Now().UTC().Format("2006-01-02")
	key := fmt.Sprintf("%s%s/%s", m.keyPrefix, date, filepath.Base(localPath))

	slog.Debug("minio: uploading media", "key", key, "path", localPath, "content_type", contentType)

	if err := m.client.UploadFile(ctx, key, localPath, contentType); err != nil {
		return "", fmt.Errorf("minio media store: %w", err)
	}

	// When bucket has public access, return a permanent URL instead of a presigned one.
	if m.client.IsPublic() {
		publicURL := m.client.PublicURL(key)
		slog.Debug("minio: media uploaded (public URL)", "key", key)
		return publicURL, nil
	}

	presignedURL, err := m.client.PresignURL(ctx, key, m.presignExpiry)
	if err != nil {
		return "", fmt.Errorf("minio media store presign: %w", err)
	}

	slog.Debug("minio: media uploaded", "key", key, "url_len", len(presignedURL))
	return presignedURL, nil
}

// inferContentType guesses MIME type from file extension, with a fallback read
// for unknown extensions.
func inferContentType(path string) string {
	ext := filepath.Ext(path)
	if ext != "" {
		if mt := mime.TypeByExtension(ext); mt != "" {
			return mt
		}
	}
	// Sample first 512 bytes for content-type sniffing
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	if n == 0 {
		return "application/octet-stream"
	}
	// Go's net/http DetectContentType is available but not imported; return generic
	return "application/octet-stream"
}
