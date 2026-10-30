package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"sis2/internal/common/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3 struct {
	presign *s3.PresignClient
	bucket  string
	public  string
}

func newClient(cfg *config.S3Config, endpoint string) (*s3.Client, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")),
	)
	if err != nil {
		return nil, err
	}
	return s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true // http://host/bucket/key: works with any S3-compatible server
	}), nil
}

// NewS3 creates the bucket if needed and makes its objects publicly readable,
// so an image URL in a post works without credentials (a CDN sits in front
// of it in SIS #1).
func NewS3(ctx context.Context, cfg *config.S3Config) (*S3, error) {
	admin, err := newClient(cfg, cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	if _, err = admin.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(cfg.Bucket)}); err != nil {
		var owned *types.BucketAlreadyOwnedByYou
		var exists *types.BucketAlreadyExists
		if !errors.As(err, &owned) && !errors.As(err, &exists) {
			return nil, fmt.Errorf("create bucket %s: %w", cfg.Bucket, err)
		}
	}
	policy := fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":["*"]},"Action":["s3:GetObject"],"Resource":["arn:aws:s3:::%s/*"]}]}`, cfg.Bucket)
	if _, err = admin.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{Bucket: aws.String(cfg.Bucket), Policy: aws.String(policy)}); err != nil {
		return nil, fmt.Errorf("public-read policy on %s: %w", cfg.Bucket, err)
	}

	// Presigning is local computation, but the signature covers the Host
	// header: sign for the host the client will actually connect to.
	public, err := newClient(cfg, cfg.PublicEndpoint)
	if err != nil {
		return nil, err
	}
	return &S3{
		presign: s3.NewPresignClient(public),
		bucket:  cfg.Bucket,
		public:  strings.TrimRight(cfg.PublicEndpoint, "/") + "/" + cfg.Bucket,
	}, nil
}

// PresignUpload signs a POST policy rather than a PUT URL: a presigned PUT
// only signs the host, so it would accept any content type and any size into
// a public bucket. The policy makes the store enforce both.
func (s *S3) PresignUpload(ctx context.Context, key, contentType string, maxBytes int64, ttl time.Duration) (Upload, string, error) {
	req, err := s.presign.PresignPostObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(o *s3.PresignPostOptions) {
		o.Expires = ttl
		o.Conditions = []interface{}{
			[]interface{}{"eq", "$Content-Type", contentType},
			[]interface{}{"content-length-range", 1, maxBytes},
		}
	})
	if err != nil {
		return Upload{}, "", err
	}
	fields := make(map[string]string, len(req.Values)+1)
	for k, v := range req.Values {
		fields[k] = v
	}
	fields["Content-Type"] = contentType
	return Upload{URL: req.URL, Fields: fields}, s.public + "/" + key, nil
}
