package tools

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/config"
	minioclient "github.com/nextlevelbuilder/goclaw/internal/storage/minio"
)

func TestMinioTools(t *testing.T) {
	// Skip if no MinIO environment variables are set
	endpoint := os.Getenv("GOCLAW_MINIO_ENDPOINT")
	accessKey := os.Getenv("GOCLAW_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("GOCLAW_MINIO_SECRET_KEY")
	bucket := os.Getenv("GOCLAW_MINIO_BUCKET")

	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		t.Skip("MinIO tools test skipped: GOCLAW_MINIO_* environment variables not set")
	}

	isPublic := os.Getenv("GOCLAW_MINIO_PUBLIC_ACCESS") == "true"
	cfg := config.MinIOConfig{
		Endpoint:     endpoint,
		AccessKey:    accessKey,
		SecretKey:    secretKey,
		Bucket:       bucket,
		PublicAccess: isPublic,
	}

	client, err := minioclient.New(cfg)
	if err != nil {
		t.Fatalf("failed to create minio client: %v", err)
	}

	ctx := context.Background()

	// 1. MinioUploadTool
	uploadTool := NewMinioUploadTool(client)
	key := "test/tool-hello-" + time.Now().Format("150405") + ".txt"
	content := "Hello from MinioUploadTool!"

	res := uploadTool.Execute(ctx, map[string]interface{}{
		"key":     key,
		"source":  "text",
		"content": content,
	})
	if res.IsError {
		t.Fatalf("MinioUploadTool failed: %v", res.ForLLM)
	}
	t.Logf("Uploaded %s", key)

	// 2. MinioPresignTool
	presignTool := NewMinioPresignTool(client)

	// Test case: Default (expires_minutes not set)
	res = presignTool.Execute(ctx, map[string]interface{}{
		"key": key,
	})
	if res.IsError {
		t.Fatalf("MinioPresignTool (default) failed: %v", res.ForLLM)
	}
	t.Logf("Presign (default) output: %s", res.ForLLM)

	// Test case: Explicit 0
	res = presignTool.Execute(ctx, map[string]interface{}{
		"key":             key,
		"expires_minutes": 0.0,
	})
	if res.IsError {
		if !isPublic {
			t.Log("Expected error (not public): " + res.ForLLM)
		} else {
			t.Fatalf("MinioPresignTool (expires_minutes=0) failed on public bucket: %v", res.ForLLM)
		}
	} else {
		if !isPublic {
			t.Error("MinioPresignTool (expires_minutes=0) should have failed on private bucket")
		} else {
			t.Logf("Presign (expires_minutes=0) output: %s", res.ForLLM)
		}
	}

	// 3. MinioDownloadTool
	downloadTool := NewMinioDownloadTool(client)
	res = downloadTool.Execute(ctx, map[string]interface{}{
		"key":    key,
		"format": "text",
	})
	if res.IsError {
		t.Fatalf("MinioDownloadTool failed: %v", res.ForLLM)
	}
	t.Logf("Downloaded content: %s", res.ForLLM)

	// 4. MinioDeleteTool
	deleteTool := NewMinioDeleteTool(client)
	res = deleteTool.Execute(ctx, map[string]interface{}{
		"key": key,
	})
	if res.IsError {
		t.Fatalf("MinioDeleteTool failed: %v", res.ForLLM)
	}
	t.Logf("Deleted %s", key)
}
