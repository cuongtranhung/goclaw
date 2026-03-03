package minio

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

func TestMinioIntegration(t *testing.T) {
	// Skip if no MinIO environment variables are set
	endpoint := os.Getenv("GOCLAW_MINIO_ENDPOINT")
	accessKey := os.Getenv("GOCLAW_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("GOCLAW_MINIO_SECRET_KEY")
	bucket := os.Getenv("GOCLAW_MINIO_BUCKET")

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		t.Skip("MinIO integration test skipped: GOCLAW_MINIO_* environment variables not set")
	}

	cfg := config.MinIOConfig{
		Endpoint:     endpoint,
		AccessKey:    accessKey,
		SecretKey:    secretKey,
		Bucket:       bucket,
		PublicAccess: os.Getenv("GOCLAW_MINIO_PUBLIC_ACCESS") == "true",
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create minio client: %v", err)
	}

	ctx := context.Background()

	// 1. Ensure Bucket
	if err := client.EnsureBucket(ctx); err != nil {
		t.Fatalf("failed to ensure bucket: %v", err)
	}

	// 2. Upload
	key := "test/hello-" + time.Now().Format("150405") + ".txt"
	content := []byte("Hello GoClaw MinIO Test!")
	if err := client.UploadBytes(ctx, key, content, "text/plain"); err != nil {
		t.Fatalf("failed to upload: %v", err)
	}
	t.Logf("Uploaded %s", key)

	// 3. Get Link (Presign or Public)
	var url string
	if client.IsPublic() {
		url = client.PublicURL(key)
		t.Logf("Public URL: %s", url)
	} else {
		url, err = client.PresignURL(ctx, key, 10*time.Minute)
		if err != nil {
			t.Fatalf("failed to presign: %v", err)
		}
		t.Logf("Presigned URL: %s", url)
	}

	// 4. Download (via HTTP GET to verify link works)
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("failed to download via URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("download failed with status: %s", resp.Status)
	}

	downloaded, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read downloaded content: %v", err)
	}

	if !bytes.Equal(content, downloaded) {
		t.Errorf("content mismatch: expected %q, got %q", string(content), string(downloaded))
	}

	// 5. Cleanup
	if err := client.Delete(ctx, key); err != nil {
		t.Errorf("failed to delete test object: %v", err)
	}
}
