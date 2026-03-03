// Package minio provides a thin wrapper around the minio-go/v7 client for use
// throughout GoClaw (agent tools + backend media store).
package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/nextlevelbuilder/goclaw/internal/config"
)

// ObjectInfo describes a MinIO object.
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ContentType  string
	ETag         string
}

// Client wraps *minio.Client with a default bucket.
type Client struct {
	mc               *minio.Client
	bucket           string
	uploadEndpoint   string // API endpoint used for upload (e.g. "host:9000")
	downloadEndpoint string // endpoint to substitute in presigned URLs (e.g. "host:9001")
	uploadSSL        bool
	downloadSSL      bool
	publicAccess     bool // bucket is publicly readable — PublicURL returns permanent links
}

// New creates an authenticated MinIO client.
// When cfg.DownloadEndpoint is set, PresignURL replaces the upload host:port
// with the download host:port so presigned URLs point to the correct port.
func New(cfg config.MinIOConfig) (*Client, error) {
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.SSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio: new client: %w", err)
	}
	dlEndpoint := cfg.DownloadEndpoint
	dlSSL := cfg.DownloadSSL
	if dlEndpoint == "" {
		dlEndpoint = cfg.Endpoint
		dlSSL = cfg.SSL
	}
	c := &Client{
		mc:               mc,
		bucket:           cfg.Bucket,
		uploadEndpoint:   cfg.Endpoint,
		downloadEndpoint: dlEndpoint,
		uploadSSL:        cfg.SSL,
		downloadSSL:      dlSSL,
		publicAccess:     cfg.PublicAccess,
	}
	return c, nil
}

// EnsureBucket creates the bucket if it does not exist.
func (c *Client) EnsureBucket(ctx context.Context) error {
	exists, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return fmt.Errorf("minio: bucket exists check: %w", err)
	}
	if !exists {
		if err := c.mc.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("minio: make bucket %q: %w", c.bucket, err)
		}
	}
	return nil
}

// Upload uploads data from reader to the given key.
// Pass size = -1 to let MinIO determine the size via multipart.
func (c *Client) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	_, err := c.mc.PutObject(ctx, c.bucket, key, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("minio: upload %q: %w", key, err)
	}
	return nil
}

// UploadBytes uploads a byte slice to the given key.
func (c *Client) UploadBytes(ctx context.Context, key string, data []byte, contentType string) error {
	if contentType == "" {
		contentType = DetectContentType(key, data)
	}
	return c.Upload(ctx, key, bytes.NewReader(data), int64(len(data)), contentType)
}

// UploadFile opens a local file and streams it to MinIO.
// contentType is inferred from the extension when empty.
func (c *Client) UploadFile(ctx context.Context, key, filePath, contentType string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("minio: open %q: %w", filePath, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("minio: stat %q: %w", filePath, err)
	}

	if contentType == "" {
		// Sample first 512 bytes for sniffing
		buf := make([]byte, 512)
		n, _ := f.ReadAt(buf, 0)
		contentType = DetectContentType(filePath, buf[:n])
	}

	return c.Upload(ctx, key, f, fi.Size(), contentType)
}

// DetectContentType returns a MIME type inferred from the filename extension
// or content sniffing (http.DetectContentType).
func DetectContentType(name string, data []byte) string {
	if ext := filepath.Ext(name); ext != "" {
		if ct := mime.TypeByExtension(ext); ct != "" {
			return ct
		}
	}
	if len(data) > 0 {
		return http.DetectContentType(data)
	}
	return "application/octet-stream"
}

// Download returns a reader for the given key plus its object metadata.
// The caller must close the returned ReadCloser.
func (c *Client) Download(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, ObjectInfo{}, fmt.Errorf("minio: download %q: %w", key, err)
	}
	st, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, ObjectInfo{}, fmt.Errorf("minio: stat %q: %w", key, err)
	}
	info := ObjectInfo{
		Key:          st.Key,
		Size:         st.Size,
		LastModified: st.LastModified,
		ContentType:  st.ContentType,
		ETag:         st.ETag,
	}
	return obj, info, nil
}

// List returns up to maxKeys objects whose key begins with prefix.
// Pass maxKeys ≤ 0 for the default (1000).
func (c *Client) List(ctx context.Context, prefix string, maxKeys int) ([]ObjectInfo, error) {
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	opts := minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}
	var objects []ObjectInfo
	for obj := range c.mc.ListObjects(ctx, c.bucket, opts) {
		if obj.Err != nil {
			return objects, fmt.Errorf("minio: list: %w", obj.Err)
		}
		objects = append(objects, ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
			ContentType:  obj.ContentType,
			ETag:         obj.ETag,
		})
		if len(objects) >= maxKeys {
			break
		}
	}
	return objects, nil
}

// Delete removes a single object.
func (c *Client) Delete(ctx context.Context, key string) error {
	if err := c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("minio: delete %q: %w", key, err)
	}
	return nil
}

// PresignURL generates a time-limited GET URL for the object.
// When a separate DownloadEndpoint is configured (e.g. port 9001 vs upload port 9000),
// the URL's host is replaced with the download endpoint so the link points to the
// correct port.
func (c *Client) PresignURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	u, err := c.mc.PresignedGetObject(ctx, c.bucket, key, expires, url.Values{})
	if err != nil {
		return "", fmt.Errorf("minio: presign %q: %w", key, err)
	}
	presigned := u.String()

	// Replace upload endpoint with download endpoint in the presigned URL.
	if c.downloadEndpoint != c.uploadEndpoint {
		uploadScheme := "http"
		if c.uploadSSL {
			uploadScheme = "https"
		}
		downloadScheme := "http"
		if c.downloadSSL {
			downloadScheme = "https"
		}
		oldPrefix := uploadScheme + "://" + c.uploadEndpoint
		newPrefix := downloadScheme + "://" + c.downloadEndpoint
		if strings.HasPrefix(presigned, oldPrefix) {
			presigned = newPrefix + presigned[len(oldPrefix):]
		}
	}

	return presigned, nil
}

// Stat returns metadata for a single object without downloading its data.
func (c *Client) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	st, err := c.mc.StatObject(ctx, c.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("minio: stat %q: %w", key, err)
	}
	return ObjectInfo{
		Key:          st.Key,
		Size:         st.Size,
		LastModified: st.LastModified,
		ContentType:  st.ContentType,
		ETag:         st.ETag,
	}, nil
}

// PublicURL returns a permanent public URL for the object.
// The URL is constructed as {scheme}://{downloadEndpoint}/{bucket}/{key}.
// Only usable when the bucket has public read access (config: public_access: true).
func (c *Client) PublicURL(key string) string {
	scheme := "http"
	if c.downloadSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s/%s", scheme, c.downloadEndpoint, c.bucket, key)
}

// IsPublic reports whether the bucket is configured for public (permanent URL) access.
func (c *Client) IsPublic() bool { return c.publicAccess }

// Bucket returns the default bucket name.
func (c *Client) Bucket() string { return c.bucket }
