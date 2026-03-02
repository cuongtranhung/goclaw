package config

// MinIOConfig holds settings for MinIO S3-compatible object storage.
// Secrets (AccessKey, SecretKey) must be set via env vars (GOCLAW_MINIO_ACCESS_KEY,
// GOCLAW_MINIO_SECRET_KEY) and never committed to config.json.
type MinIOConfig struct {
	Endpoint         string `json:"endpoint,omitempty"`          // upload/API endpoint, e.g. "minio.example.com:9000"
	DownloadEndpoint string `json:"download_endpoint,omitempty"` // presigned URL endpoint if different, e.g. "minio.example.com:9001"
	AccessKey        string `json:"access_key,omitempty"`        // env: GOCLAW_MINIO_ACCESS_KEY
	SecretKey        string `json:"secret_key,omitempty"`        // env: GOCLAW_MINIO_SECRET_KEY
	Bucket           string `json:"bucket,omitempty"`            // default bucket name
	SSL              bool   `json:"ssl,omitempty"`               // use HTTPS/TLS for upload endpoint
	DownloadSSL      bool   `json:"download_ssl,omitempty"`      // use HTTPS/TLS for download/presigned URLs
	Region           string `json:"region,omitempty"`            // optional region (e.g. "us-east-1")
	PublicAccess     bool   `json:"public_access,omitempty"`     // bucket is publicly readable — use permanent URLs instead of presigned

	// MediaStore controls automatic upload of agent-generated media (TTS audio,
	// generated images) to MinIO so channels receive presigned URLs instead of
	// local temp file paths.
	MediaStore MinIOMediaStoreConfig `json:"media_store,omitempty"`
}

// MinIOMediaStoreConfig controls the backend media-upload behaviour.
type MinIOMediaStoreConfig struct {
	// Enabled: when true, TTS/image temp files are automatically uploaded to
	// MinIO before being sent to channels (Telegram, etc.).
	Enabled bool `json:"enabled,omitempty"`

	// PresignExpiry is the presigned URL TTL in minutes (default 1440 = 24 h).
	PresignExpiry int `json:"presign_expiry,omitempty"`

	// KeyPrefix is the object key prefix for auto-uploaded media (default "media/").
	KeyPrefix string `json:"key_prefix,omitempty"`
}

// IsEnabled reports whether MinIO is fully configured (endpoint + credentials + bucket).
func (c MinIOConfig) IsEnabled() bool {
	return c.Endpoint != "" && c.AccessKey != "" && c.SecretKey != "" && c.Bucket != ""
}
