package upload

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
)

// MultipartOptions configure a multipart upload.
type MultipartOptions struct {
	// PartSizeBytes is the size of each part. Defaults to 16MB.
	PartSizeBytes int64
	// Concurrency is the number of parts to upload in parallel. Default 5.
	Concurrency int
}

// PutOriginal uploads a (potentially gigabyte-scale) file to the given key
// using multipart. Cache-Control is forced to "private, max-age=0" — originals
// live under originals/ and are archival, not hot.
func (c *Client) PutOriginal(ctx context.Context, localPath, key string, opts MultipartOptions) error {
	if opts.PartSizeBytes == 0 {
		opts.PartSizeBytes = 16 * 1024 * 1024
	}
	if opts.Concurrency == 0 {
		opts.Concurrency = 5
	}
	f, err := os.Open(localPath)
	if err != nil {
		return problems.New("upload/object-failed", "Could not open original",
			err.Error(), problems.WithExt(problems.Ext("path", localPath)))
	}
	defer f.Close()

	up := manager.NewUploader(c.s3, func(u *manager.Uploader) {
		u.PartSize = opts.PartSizeBytes
		u.Concurrency = opts.Concurrency
	})

	_, err = up.Upload(ctx, &s3.PutObjectInput{
		Bucket:       aws.String(c.bucket),
		Key:          aws.String(key),
		Body:         f,
		CacheControl: aws.String(privateCacheControl),
		ContentType:  aws.String(contentTypeFor(key)),
	})
	if err != nil {
		return wrapPutErr(err, key)
	}
	return nil
}
