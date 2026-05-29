// Package upload talks to Cloudflare R2 over the S3-compatible API. It exposes
// a directory uploader for tiles/thumbnails plus a multipart helper for the
// gigabyte-scale original TIFFs.
//
// Two design choices worth knowing:
//   - We set Cache-Control per-object based on the destination key extension.
//     Tiles are forever-immutable, info.json is short-cached so reprocesses
//     show up quickly, originals are private.
//   - We assert the Cache-Control header round-tripped after upload by HEAD-ing
//     info.json. R2 silently drops unknown headers; the HEAD check catches
//     misconfigured PUTs before users hit a stale cache.
package upload

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
)

// Credentials wires R2 access into the S3-compatible client.
type Credentials struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
}

// Client is a thin S3-API wrapper holding the configured bucket.
type Client struct {
	s3     *s3.Client
	bucket string
}

// NewClient builds an R2 client. The endpoint is derived from the account ID;
// the region is the literal "auto" per Cloudflare's docs.
func NewClient(ctx context.Context, creds Credentials) (*Client, error) {
	if creds.AccountID == "" || creds.AccessKeyID == "" || creds.SecretAccessKey == "" || creds.Bucket == "" {
		return nil, problems.New(
			"upload/auth-failed",
			"R2 credentials are incomplete",
			"AccountID, AccessKeyID, SecretAccessKey, and Bucket are all required",
		)
	}
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", creds.AccountID)
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion("auto"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			creds.AccessKeyID, creds.SecretAccessKey, "",
		)),
	)
	if err != nil {
		return nil, problems.New(
			"upload/auth-failed",
			"Could not configure AWS SDK",
			err.Error(),
		)
	}
	c := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	return &Client{s3: c, bucket: creds.Bucket}, nil
}

// Bucket returns the configured bucket name.
func (c *Client) Bucket() string { return c.bucket }

// S3 returns the underlying S3 client. Exposed so internal callers can perform
// operations (HEAD, ListObjects) not wrapped at the package level yet.
func (c *Client) S3() *s3.Client { return c.s3 }
