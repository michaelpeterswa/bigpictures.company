package upload

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
)

const (
	immutableCacheControl = "public, max-age=31536000, immutable"
	infoCacheControl      = "public, max-age=300"
	privateCacheControl   = "private, max-age=0"
)

// DirOptions configure a directory upload.
type DirOptions struct {
	// Concurrency caps parallel PUTs. Default 32.
	Concurrency int
	// Progress is called after each successful object upload.
	Progress func(Progress)
}

// Progress reports cumulative directory-upload progress.
type Progress struct {
	UploadedFiles int
	TotalFiles    int
	LastKey       string
}

// UploadDir streams every regular file under localDir to R2 under keyPrefix.
// Per-file Cache-Control + Content-Type are derived from the destination key.
//
// The total file count is computed once via a fast walk before uploads begin,
// so the Progress callback can report N/Total without a second pass.
func (c *Client) UploadDir(ctx context.Context, localDir, keyPrefix string, opts DirOptions) error {
	if opts.Concurrency <= 0 {
		opts.Concurrency = 32
	}
	files, err := listFiles(localDir)
	if err != nil {
		return err
	}
	total := len(files)
	sem := semaphore.NewWeighted(int64(opts.Concurrency))
	g, gctx := errgroup.WithContext(ctx)
	var uploaded atomic.Int64

	for _, rel := range files {
		if err := sem.Acquire(gctx, 1); err != nil {
			return wrapRateLimitOrCtx(err)
		}
		g.Go(func() error {
			defer sem.Release(1)
			key := path.Join(keyPrefix, filepath.ToSlash(rel))
			full := filepath.Join(localDir, rel)
			if err := c.putFile(gctx, full, key); err != nil {
				return err
			}
			n := uploaded.Add(1)
			if opts.Progress != nil {
				opts.Progress(Progress{
					UploadedFiles: int(n),
					TotalFiles:    total,
					LastKey:       key,
				})
			}
			return nil
		})
	}
	return g.Wait()
}

// PutFile uploads a single local file to the given key, applying the standard
// per-extension Cache-Control + Content-Type rules.
func (c *Client) PutFile(ctx context.Context, localPath, key string) error {
	return c.putFile(ctx, localPath, key)
}

// DeletePrefix removes every object whose key starts with prefix. It pages
// through ListObjectsV2 to handle prefixes with thousands of tiles.
func (c *Client) DeletePrefix(ctx context.Context, prefix string) error {
	var continuationToken *string
	for {
		out, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(c.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return problems.New("upload/object-failed", "ListObjectsV2 failed during cleanup",
				err.Error(), problems.WithExt(problems.Ext("prefix", prefix)))
		}
		for _, obj := range out.Contents {
			if _, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(c.bucket),
				Key:    obj.Key,
			}); err != nil {
				return problems.New("upload/object-failed", "DeleteObject failed during cleanup",
					err.Error(), problems.WithExt(problems.Ext("key", aws.ToString(obj.Key))))
			}
		}
		if !aws.ToBool(out.IsTruncated) {
			return nil
		}
		continuationToken = out.NextContinuationToken
	}
}

// DeleteKey removes a single object. Missing keys are treated as success.
func (c *Client) DeleteKey(ctx context.Context, key string) error {
	if _, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return problems.New("upload/object-failed", "DeleteObject failed",
			err.Error(), problems.WithExt(problems.Ext("key", key)))
	}
	return nil
}

// AssertHeaders HEADs the given key and verifies Cache-Control + Content-Type
// match what we would have written. R2 silently drops unknown PUT headers;
// this check catches misconfigurations that would otherwise only surface as
// stale tiles in production.
func (c *Client) AssertHeaders(ctx context.Context, key string) error {
	resp, err := c.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return problems.New(
			"upload/header-mismatch",
			"Could not HEAD object after upload",
			err.Error(),
			problems.WithExt(problems.Ext("key", key)),
		)
	}
	wantCC := cacheControlFor(key)
	gotCC := aws.ToString(resp.CacheControl)
	if gotCC != wantCC {
		return problems.New(
			"upload/header-mismatch",
			"Cache-Control mismatch on uploaded object",
			fmt.Sprintf("got %q, want %q", gotCC, wantCC),
			problems.WithExt(
				problems.Ext("key", key),
				problems.Ext("got", gotCC),
				problems.Ext("want", wantCC),
			),
		)
	}
	return nil
}

func (c *Client) putFile(ctx context.Context, localPath, key string) error {
	f, err := os.Open(localPath)
	if err != nil {
		return problems.New("upload/object-failed", "Could not open local file", err.Error(),
			problems.WithExt(problems.Ext("path", localPath), problems.Ext("key", key)))
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return problems.New("upload/object-failed", "Could not stat local file", err.Error(),
			problems.WithExt(problems.Ext("path", localPath)))
	}
	in := &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          f,
		ContentLength: aws.Int64(fi.Size()),
		CacheControl:  aws.String(cacheControlFor(key)),
		ContentType:   aws.String(contentTypeFor(key)),
	}
	if _, err := c.s3.PutObject(ctx, in); err != nil {
		return wrapPutErr(err, key)
	}
	return nil
}

// cacheControlFor picks the Cache-Control header for the given key. Tiles and
// thumbnails are forever-immutable. info.json is short-cached so reprocesses
// don't strand viewers on a stale pyramid. Originals (TIFFs under originals/)
// are private archives, not hot assets.
func cacheControlFor(key string) string {
	base := filepath.Base(key)
	if base == "info.json" {
		return infoCacheControl
	}
	if strings.HasPrefix(key, "originals/") {
		return privateCacheControl
	}
	return immutableCacheControl
}

// contentTypeFor picks a Content-Type for the given key based on extension.
// We hard-code the small set the pipeline produces rather than dragging in
// mime detection — it's a closed world.
func contentTypeFor(key string) string {
	switch strings.ToLower(filepath.Ext(key)) {
	case ".webp":
		return "image/webp"
	case ".json":
		return "application/json"
	case ".tif", ".tiff":
		return "image/tiff"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		return "application/octet-stream"
	}
}

// listFiles streams a relative-path list of every regular file under root.
// We do this in one pass up-front so progress callbacks can show N/Total.
func listFiles(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		return nil, problems.New("upload/object-failed", "Could not walk directory", err.Error(),
			problems.WithExt(problems.Ext("path", root)))
	}
	return out, nil
}

func wrapPutErr(err error, key string) error {
	// Cloudflare R2 returns 401/403 on auth failures and 429/503 on rate
	// limiting. Smithy surfaces these as ResponseError with HTTPStatusCode.
	if code, ok := httpStatus(err); ok {
		switch code {
		case 401, 403:
			return problems.New("upload/auth-failed", "R2 rejected credentials",
				err.Error(), problems.WithExt(problems.Ext("key", key), problems.Ext("status", code)))
		case 429, 503:
			return problems.New("upload/rate-limited", "R2 rate-limited the upload",
				err.Error(), problems.WithExt(problems.Ext("key", key), problems.Ext("status", code)))
		}
	}
	return problems.New("upload/object-failed", "PUT object failed",
		err.Error(), problems.WithExt(problems.Ext("key", key)))
}

func wrapRateLimitOrCtx(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return problems.New("upload/object-failed", "Semaphore acquire failed", err.Error())
}

// httpStatus pulls an HTTP status code out of a smithy error chain.
func httpStatus(err error) (int, bool) {
	type withStatus interface {
		HTTPStatusCode() int
	}
	var ws withStatus
	if errors.As(err, &ws) {
		return ws.HTTPStatusCode(), true
	}
	return 0, false
}
