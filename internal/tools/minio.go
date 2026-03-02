package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	minioclient "github.com/nextlevelbuilder/goclaw/internal/storage/minio"
)

// ─────────────────────────────────────────────────────────────────────────────
// minio_upload
// ─────────────────────────────────────────────────────────────────────────────

// MinioUploadTool uploads a local file or text content to MinIO.
type MinioUploadTool struct{ client *minioclient.Client }

func NewMinioUploadTool(c *minioclient.Client) *MinioUploadTool { return &MinioUploadTool{client: c} }
func (t *MinioUploadTool) Name() string                          { return "minio_upload" }
func (t *MinioUploadTool) Description() string {
	return "Upload a local file or text content to MinIO object storage. Returns the object key on success."
}

func (t *MinioUploadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Destination object key (path) in the bucket, e.g. \"reports/2024/result.json\".",
			},
			"source": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"file", "text"},
				"description": "\"file\" to upload a local file; \"text\" to upload inline text content.",
			},
			"file_path": map[string]interface{}{
				"type":        "string",
				"description": "Absolute path of the local file to upload. Required when source=\"file\".",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "Text content to upload. Required when source=\"text\".",
			},
			"content_type": map[string]interface{}{
				"type":        "string",
				"description": "MIME type, e.g. \"application/json\" or \"text/plain\". Optional — inferred from extension when omitted.",
			},
		},
		"required": []string{"key", "source"},
	}
}

func (t *MinioUploadTool) Execute(ctx context.Context, args map[string]interface{}) *Result {
	key, _ := args["key"].(string)
	source, _ := args["source"].(string)
	contentType, _ := args["content_type"].(string)

	if key == "" {
		return ErrorResult("minio_upload: key is required")
	}

	switch source {
	case "file":
		filePath, _ := args["file_path"].(string)
		if filePath == "" {
			return ErrorResult("minio_upload: file_path is required when source=\"file\"")
		}
		if err := t.client.UploadFile(ctx, key, filePath, contentType); err != nil {
			return ErrorResult(fmt.Sprintf("minio_upload: %v", err))
		}

	case "text":
		content, _ := args["content"].(string)
		if content == "" {
			return ErrorResult("minio_upload: content is required when source=\"text\"")
		}
		if contentType == "" {
			contentType = "text/plain; charset=utf-8"
		}
		if err := t.client.UploadBytes(ctx, key, []byte(content), contentType); err != nil {
			return ErrorResult(fmt.Sprintf("minio_upload: %v", err))
		}

	default:
		return ErrorResult("minio_upload: source must be \"file\" or \"text\"")
	}

	return NewResult(fmt.Sprintf("Uploaded successfully. Key: %s (bucket: %s)", key, t.client.Bucket()))
}

// ─────────────────────────────────────────────────────────────────────────────
// minio_download
// ─────────────────────────────────────────────────────────────────────────────

// MinioDownloadTool downloads an object from MinIO.
// format="text" → returns content as text (default for small files)
// format="file" → saves to /tmp and returns MEDIA:/tmp/... path
type MinioDownloadTool struct{ client *minioclient.Client }

func NewMinioDownloadTool(c *minioclient.Client) *MinioDownloadTool {
	return &MinioDownloadTool{client: c}
}
func (t *MinioDownloadTool) Name() string { return "minio_download" }
func (t *MinioDownloadTool) Description() string {
	return "Download an object from MinIO. Use format=\"text\" to return the content directly (default), or format=\"file\" to save it as a local file and return the path."
}

func (t *MinioDownloadTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Object key to download.",
			},
			"format": map[string]interface{}{
				"type":        "string",
				"enum":        []string{"text", "file"},
				"description": "\"text\" (default) returns content inline; \"file\" saves to /tmp and returns the path.",
			},
		},
		"required": []string{"key"},
	}
}

func (t *MinioDownloadTool) Execute(ctx context.Context, args map[string]interface{}) *Result {
	key, _ := args["key"].(string)
	format, _ := args["format"].(string)
	if format == "" {
		format = "text"
	}
	if key == "" {
		return ErrorResult("minio_download: key is required")
	}

	reader, info, err := t.client.Download(ctx, key)
	if err != nil {
		return ErrorResult(fmt.Sprintf("minio_download: %v", err))
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return ErrorResult(fmt.Sprintf("minio_download: read: %v", err))
	}

	if format == "file" {
		ext := filepath.Ext(key)
		tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("minio-%d%s", time.Now().UnixNano(), ext))
		if err := os.WriteFile(tmpPath, data, 0644); err != nil {
			return ErrorResult(fmt.Sprintf("minio_download: write temp: %v", err))
		}
		return NewResult(fmt.Sprintf("MEDIA:%s", tmpPath))
	}

	// text mode
	content := string(data)
	summary := fmt.Sprintf("Downloaded %q — size: %d bytes, content-type: %s\n\n%s",
		key, info.Size, info.ContentType, content)
	return NewResult(summary)
}

// ─────────────────────────────────────────────────────────────────────────────
// minio_list
// ─────────────────────────────────────────────────────────────────────────────

// MinioListTool lists objects in the default bucket under an optional prefix.
type MinioListTool struct{ client *minioclient.Client }

func NewMinioListTool(c *minioclient.Client) *MinioListTool { return &MinioListTool{client: c} }
func (t *MinioListTool) Name() string                        { return "minio_list" }
func (t *MinioListTool) Description() string {
	return "List objects in the MinIO bucket. Use the prefix parameter to narrow results to a specific path."
}

func (t *MinioListTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"prefix": map[string]interface{}{
				"type":        "string",
				"description": "Key prefix to filter results, e.g. \"reports/2024/\". Empty returns all objects.",
			},
			"max_objects": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum number of objects to return (default 100, max 1000).",
			},
		},
		"required": []string{},
	}
}

func (t *MinioListTool) Execute(ctx context.Context, args map[string]interface{}) *Result {
	prefix, _ := args["prefix"].(string)
	maxObjs := 100
	if v, ok := args["max_objects"].(float64); ok && v > 0 {
		maxObjs = int(v)
		if maxObjs > 1000 {
			maxObjs = 1000
		}
	}

	objects, err := t.client.List(ctx, prefix, maxObjs)
	if err != nil {
		return ErrorResult(fmt.Sprintf("minio_list: %v", err))
	}

	if len(objects) == 0 {
		return NewResult(fmt.Sprintf("No objects found in bucket %q with prefix %q.", t.client.Bucket(), prefix))
	}

	type jsonObj struct {
		Key          string `json:"key"`
		Size         int64  `json:"size"`
		LastModified string `json:"last_modified"`
		ContentType  string `json:"content_type,omitempty"`
	}
	list := make([]jsonObj, len(objects))
	for i, o := range objects {
		list[i] = jsonObj{
			Key:          o.Key,
			Size:         o.Size,
			LastModified: o.LastModified.UTC().Format(time.RFC3339),
			ContentType:  o.ContentType,
		}
	}

	data, _ := json.MarshalIndent(map[string]interface{}{
		"bucket":  t.client.Bucket(),
		"prefix":  prefix,
		"count":   len(list),
		"objects": list,
	}, "", "  ")
	return NewResult(string(data))
}

// ─────────────────────────────────────────────────────────────────────────────
// minio_delete
// ─────────────────────────────────────────────────────────────────────────────

// MinioDeleteTool removes a single object from MinIO.
type MinioDeleteTool struct{ client *minioclient.Client }

func NewMinioDeleteTool(c *minioclient.Client) *MinioDeleteTool { return &MinioDeleteTool{client: c} }
func (t *MinioDeleteTool) Name() string                          { return "minio_delete" }
func (t *MinioDeleteTool) Description() string {
	return "Delete an object from MinIO by key."
}

func (t *MinioDeleteTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Object key to delete.",
			},
		},
		"required": []string{"key"},
	}
}

func (t *MinioDeleteTool) Execute(ctx context.Context, args map[string]interface{}) *Result {
	key, _ := args["key"].(string)
	if key == "" {
		return ErrorResult("minio_delete: key is required")
	}
	if err := t.client.Delete(ctx, key); err != nil {
		return ErrorResult(fmt.Sprintf("minio_delete: %v", err))
	}
	return NewResult(fmt.Sprintf("Deleted %q from bucket %q.", key, t.client.Bucket()))
}

// ─────────────────────────────────────────────────────────────────────────────
// minio_presign
// ─────────────────────────────────────────────────────────────────────────────

// MinioPresignTool generates a time-limited presigned GET URL for an object.
type MinioPresignTool struct{ client *minioclient.Client }

func NewMinioPresignTool(c *minioclient.Client) *MinioPresignTool {
	return &MinioPresignTool{client: c}
}
func (t *MinioPresignTool) Name() string { return "minio_presign" }
func (t *MinioPresignTool) Description() string {
	return "Generate a time-limited presigned GET URL for a MinIO object. Anyone with the URL can download the object without credentials."
}

func (t *MinioPresignTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"key": map[string]interface{}{
				"type":        "string",
				"description": "Object key to generate URL for.",
			},
			"expires_minutes": map[string]interface{}{
				"type":        "integer",
				"description": "URL validity in minutes (default 60, max 10080 = 7 days).",
			},
		},
		"required": []string{"key"},
	}
}

func (t *MinioPresignTool) Execute(ctx context.Context, args map[string]interface{}) *Result {
	key, _ := args["key"].(string)
	if key == "" {
		return ErrorResult("minio_presign: key is required")
	}

	expiryMin := 60
	if v, ok := args["expires_minutes"].(float64); ok && v >= 0 {
		expiryMin = int(v)
		if expiryMin > 10080 {
			expiryMin = 10080
		}
	}

	// expires_minutes=0 → return permanent public URL (requires public_access=true on bucket)
	if expiryMin == 0 {
		if !t.client.IsPublic() {
			return ErrorResult("minio_presign: expires_minutes=0 (permanent URL) requires public_access=true in MinIO config")
		}
		publicURL := t.client.PublicURL(key)
		return NewResult(fmt.Sprintf("Permanent public URL for %q:\n\n%s", key, publicURL))
	}

	presignedURL, err := t.client.PresignURL(ctx, key, time.Duration(expiryMin)*time.Minute)
	if err != nil {
		return ErrorResult(fmt.Sprintf("minio_presign: %v", err))
	}

	expiresAt := time.Now().Add(time.Duration(expiryMin) * time.Minute).UTC().Format(time.RFC3339)
	result := fmt.Sprintf("Presigned URL for %q (expires in %d minutes, at %s):\n\n%s",
		key, expiryMin, expiresAt, presignedURL)

	if !strings.Contains(presignedURL, "\n") {
		result += "\n\nURL only:\n" + presignedURL
	}
	return NewResult(result)
}
