package providers

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/Radiushina/avatar-service/internal/config"
	"github.com/Radiushina/avatar-service/internal/domains/avatars"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

const maxAvatarBytes = 10 << 20

type S3Store struct {
	client *s3.Client
	bucket string
}

func NewS3Store(cfg *config.Config) *S3Store {
	region := cfg.S3.Region
	if region == "" {
		region = "us-east-1"
	}
	return &S3Store{
		bucket: cfg.S3.Bucket,
		client: s3.New(s3.Options{
			Region:       region,
			BaseEndpoint: aws.String(cfg.S3.Endpoint),
			Credentials:  credentials.NewStaticCredentialsProvider(cfg.S3.AccessKey, cfg.S3.SecretKey, ""),
			UsePathStyle: cfg.S3.PathStyle,
		}),
	}
}

func (s *S3Store) Get(ctx context.Context, key string) (body []byte, err error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isMissingObject(err) {
			return nil, avatars.ErrNotFound
		}
		return nil, fmt.Errorf("get avatar object: %w", err)
	}
	defer func() {
		closeErr := out.Body.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("close avatar object: %w", closeErr)
		}
	}()

	body, err = io.ReadAll(io.LimitReader(out.Body, maxAvatarBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read avatar object: %w", err)
	}
	if len(body) > maxAvatarBytes {
		return nil, errors.New("avatar object exceeds 10MB")
	}
	return body, nil
}

func isMissingObject(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound":
			return true
		}
	}
	var respErr *smithyhttp.ResponseError
	return errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404
}
