// Package s3 adapts S3-compatible object storage without leaking SDK types into domain.
package s3

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint, PublicEndpoint, Region, Bucket, AccessKey, SecretKey string
	CreateBucket                                                   bool
}
type Store struct {
	client, signer *minio.Client
	bucket         string
}

func New(ctx context.Context, c Config) (*Store, error) {
	if c.Bucket == "" || c.AccessKey == "" || c.SecretKey == "" {
		return nil, fmt.Errorf("object storage requires bucket and credentials")
	}
	if c.Region == "" {
		c.Region = "us-east-1"
	}
	clientFor := func(endpoint string) (*minio.Client, error) {
		u, err := url.Parse(endpoint)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return nil, fmt.Errorf("invalid S3 endpoint")
		}
		return minio.New(u.Host, &minio.Options{Creds: credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""), Secure: u.Scheme == "https", Region: c.Region})
	}
	client, err := clientFor(c.Endpoint)
	if err != nil {
		return nil, err
	}
	signer := client
	if c.PublicEndpoint != "" {
		signer, err = clientFor(c.PublicEndpoint)
		if err != nil {
			return nil, err
		}
	}
	if c.CreateBucket {
		exists, e := client.BucketExists(ctx, c.Bucket)
		if e != nil {
			return nil, fmt.Errorf("cannot access artifact bucket")
		}
		if !exists {
			if e = client.MakeBucket(ctx, c.Bucket, minio.MakeBucketOptions{Region: c.Region}); e != nil {
				return nil, fmt.Errorf("cannot create artifact bucket")
			}
		}
	}
	return &Store{client: client, signer: signer, bucket: c.Bucket}, nil
}

func safeKey(key string) bool {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return false
	}
	for _, p := range strings.Split(key, "/") {
		if p == "" || p == "." || p == ".." {
			return false
		}
	}
	return true
}
func (s *Store) Put(ctx context.Context, key string, body io.Reader, size int64) (string, error) {
	if !safeKey(key) || size <= 0 {
		return "", fmt.Errorf("invalid object key or size")
	}
	info, err := s.client.PutObject(ctx, s.bucket, key, body, size, minio.PutObjectOptions{ContentType: "application/octet-stream"})
	if err != nil || info.Size != size {
		return "", fmt.Errorf("object upload failed")
	}
	return (&url.URL{Scheme: "s3", Host: s.bucket, Path: "/" + key}).String(), nil
}
func (s *Store) DownloadURL(ctx context.Context, uri string, expires time.Duration) (string, error) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "s3" || u.Host != s.bucket || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !safeKey(strings.TrimPrefix(u.Path, "/")) {
		return "", fmt.Errorf("invalid artifact URI")
	}
	query := url.Values{"response-content-disposition": {"attachment"}}
	signed, err := s.signer.PresignedGetObject(ctx, s.bucket, strings.TrimPrefix(u.Path, "/"), expires, query)
	if err != nil {
		return "", fmt.Errorf("cannot sign artifact download")
	}
	return signed.String(), nil
}
