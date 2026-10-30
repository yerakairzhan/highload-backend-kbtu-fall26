package config

import "time"

type LogConfig struct {
	Level string `yaml:"level" default:"INFO"`
}

type SeedConfig struct {
	Enabled         bool   `yaml:"enabled"           default:"true"`
	Posts           int    `yaml:"posts"             default:"1000"`
	CommentsPerPost int    `yaml:"comments_per_post" default:"50"`
	Users           int    `yaml:"users"             default:"100"`
	UserPassword    string `yaml:"user_password"     default:"lab-password"` // every seeded user's password (lab only)
}

// JWTConfig signs access tokens (HS256). The secret is overridden by the
// JWT_SECRET environment variable when it is set.
type JWTConfig struct {
	Secret     string        `yaml:"secret"      default:"lab-only-secret-change-me"`
	Issuer     string        `yaml:"issuer"      default:"blog-platform"`
	AccessTTL  time.Duration `yaml:"access_ttl"  default:"15m"`
	RefreshTTL time.Duration `yaml:"refresh_ttl" default:"720h"`
}

// S3Config points at the object store holding images. Endpoint is what the
// server dials; PublicEndpoint is what presigned and image URLs carry, since a
// browser cannot resolve the in-cluster host name.
type S3Config struct {
	Enabled        bool          `yaml:"enabled"`
	Endpoint       string        `yaml:"endpoint"        default:"http://localhost:9000"`
	PublicEndpoint string        `yaml:"public_endpoint" default:"http://localhost:9000"`
	Region         string        `yaml:"region"          default:"us-east-1"`
	AccessKey      string        `yaml:"access_key"`
	SecretKey      string        `yaml:"secret_key"`
	Bucket         string        `yaml:"bucket"          default:"images"`
	PresignTTL     time.Duration `yaml:"presign_ttl"     default:"10m"`
	MaxUploadBytes int64         `yaml:"max_upload_bytes" default:"5242880"` // 5 MB per image
}

type BlogPlatformConfig struct {
	Log        LogConfig     `yaml:"log"`
	ServePort  string        `yaml:"port"        default:"8080"`
	RpcTimeout time.Duration `yaml:"rpc_timeout" default:"2s"` // server-side cap on every RPC
	Seed       SeedConfig    `yaml:"seed"`
	JWT        JWTConfig     `yaml:"jwt"`
	S3         S3Config      `yaml:"s3"`
}

func GetBlogPlatformConfig() *BlogPlatformConfig {
	return GetConfig[BlogPlatformConfig]()
}
