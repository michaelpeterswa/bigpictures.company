// These tests run against a real Cloudflare R2 bucket. Set the PANO_TEST_R2_*
// variables to enable; they are skipped otherwise. They write to and delete
// from the bucket — point at a throwaway test bucket only.
package upload

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func testClient(t *testing.T) (*Client, string) {
	t.Helper()
	account := os.Getenv("PANO_TEST_R2_ACCOUNT_ID")
	ak := os.Getenv("PANO_TEST_R2_ACCESS_KEY_ID")
	sk := os.Getenv("PANO_TEST_R2_SECRET_ACCESS_KEY")
	bucket := os.Getenv("PANO_TEST_R2_BUCKET")
	if account == "" || ak == "" || sk == "" || bucket == "" {
		t.Skip("PANO_TEST_R2_* not set")
	}
	c, err := NewClient(context.Background(), Credentials{
		AccountID:       account,
		AccessKeyID:     ak,
		SecretAccessKey: sk,
		Bucket:          bucket,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	prefix := "pano-test/" + t.Name()
	t.Cleanup(func() { cleanupPrefix(context.Background(), c, prefix) })
	return c, prefix
}

func cleanupPrefix(ctx context.Context, c *Client, prefix string) {
	out, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucket),
		Prefix: aws.String(prefix),
	})
	if err != nil {
		return
	}
	for _, obj := range out.Contents {
		_, _ = c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(c.bucket),
			Key:    obj.Key,
		})
	}
}

func TestUploadDir_Integration(t *testing.T) {
	c, prefix := testClient(t)
	ctx := context.Background()

	dir := t.TempDir()
	// Synthesize a small tile-like tree: info.json + a few .webp files.
	if err := os.WriteFile(filepath.Join(dir, "info.json"), []byte(`{"width":1,"height":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"0,0,256,256", "256,0,256,256"} {
		full := filepath.Join(dir, sub)
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(full, "0.webp"), bytes.Repeat([]byte{0xff}, 32), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var lastKey string
	if err := c.UploadDir(ctx, dir, prefix, DirOptions{
		Progress: func(p Progress) { lastKey = p.LastKey },
	}); err != nil {
		t.Fatalf("UploadDir: %v", err)
	}
	if lastKey == "" {
		t.Errorf("no progress events received")
	}

	// Headers on info.json should be the short-cache variant.
	if err := c.AssertHeaders(ctx, prefix+"/info.json"); err != nil {
		t.Errorf("AssertHeaders info.json: %v", err)
	}
	// Headers on a tile should be the immutable variant.
	tileKey := prefix + "/0,0,256,256/0.webp"
	resp, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(c.bucket), Key: aws.String(tileKey)})
	if err != nil {
		t.Fatalf("HeadObject tile: %v", err)
	}
	if got := aws.ToString(resp.CacheControl); !strings.Contains(got, "immutable") {
		t.Errorf("tile Cache-Control = %q, want immutable", got)
	}
	if got := aws.ToString(resp.ContentType); got != "image/webp" {
		t.Errorf("tile Content-Type = %q, want image/webp", got)
	}
}

func TestCacheControlFor(t *testing.T) {
	cases := []struct {
		key  string
		want string
	}{
		{"panos/foo/info.json", infoCacheControl},
		{"panos/foo/0/0/0/0.webp", immutableCacheControl},
		{"panos/foo/thumb-300.webp", immutableCacheControl},
		{"originals/foo.tiff", privateCacheControl},
	}
	for _, c := range cases {
		if got := cacheControlFor(c.key); got != c.want {
			t.Errorf("cacheControlFor(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}

func TestContentTypeFor(t *testing.T) {
	cases := []struct{ key, want string }{
		{"a/b.webp", "image/webp"},
		{"a/b.json", "application/json"},
		{"a/b.tiff", "image/tiff"},
		{"a/b", "application/octet-stream"},
	}
	for _, c := range cases {
		if got := contentTypeFor(c.key); got != c.want {
			t.Errorf("contentTypeFor(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}
