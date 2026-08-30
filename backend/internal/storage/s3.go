// Package storage wraps S3 (or an S3-compatible endpoint, e.g. MinIO for
// local dev) for the product-photo upload flow described in brief §7.
package storage

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type Client struct {
	s3        *s3.Client
	presigner *s3.PresignClient
	bucket    string
	publicURL string // base URL for GETs, e.g. "http://localhost:9000/equiptra-product-photos" or a CloudFront/S3 URL
}

// NewClient builds a Client from environment variables. Returns (nil, nil)
// if S3_BUCKET is unset — the photo feature is simply disabled rather than
// treated as an error, since it's v1.1/optional per the brief.
//
//	S3_BUCKET           required to enable the feature
//	AWS_REGION          default "us-east-1"
//	S3_ENDPOINT_URL     optional — set for MinIO/local dev (path-style addressing)
//	S3_PUBLIC_BASE_URL  optional override for constructing public GET URLs;
//	                    defaults to a sensible value derived from the endpoint/bucket/region
//
// AWS credentials come from the SDK's standard chain (env vars, shared
// config/credentials files, or an instance/task role in AWS).
func NewClient(ctx context.Context) (*Client, error) {
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		return nil, nil
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	endpoint := os.Getenv("S3_ENDPOINT_URL")

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("loading AWS config: %w", err)
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // required for MinIO and most non-AWS S3-compatible servers
		}
	})

	publicURL := os.Getenv("S3_PUBLIC_BASE_URL")
	if publicURL == "" {
		if endpoint != "" {
			publicURL = strings.TrimSuffix(endpoint, "/") + "/" + bucket
		} else {
			publicURL = fmt.Sprintf("https://%s.s3.%s.amazonaws.com", bucket, region)
		}
	}

	return &Client{
		s3:        s3Client,
		presigner: s3.NewPresignClient(s3Client),
		bucket:    bucket,
		publicURL: strings.TrimSuffix(publicURL, "/"),
	}, nil
}

// PresignPutURL returns a short-lived URL the browser can PUT the file
// contents to directly, and the public GET URL the object will be
// reachable at afterwards (to store as products.image_url once the upload
// succeeds).
func (c *Client) PresignPutURL(ctx context.Context, key, contentType string) (putURL, getURL string, err error) {
	req, err := c.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(10*time.Minute))
	if err != nil {
		return "", "", fmt.Errorf("presigning put: %w", err)
	}
	return req.URL, c.publicURL + "/" + key, nil
}

// PutObject uploads directly — used by the one-off photo migration tool,
// where there's no browser round-trip to presign for.
func (c *Client) PutObject(ctx context.Context, key, contentType string, body []byte) (getURL string, err error) {
	_, err = c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(body),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("putting object %s: %w", key, err)
	}
	return c.publicURL + "/" + key, nil
}
