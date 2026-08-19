package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// R2Client はCloudflare R2（S3互換API）への署名付きアップロードURL発行・
// オブジェクト削除を行う薄いラッパー。
type R2Client struct {
	presign *s3.PresignClient
	client  *s3.Client
	bucket  string
}

// NewR2Client はaccountID/accessKeyID/secretAccessKey/bucketのいずれかが空文字なら
// nilを返す（未設定時はハンドラ側で500 storage_not_configuredを返す方針のため。
// docs/aibo/m4-implementation-plan.md参照）。
func NewR2Client(accountID, accessKeyID, secretAccessKey, bucket string) *R2Client {
	if accountID == "" || accessKeyID == "" || secretAccessKey == "" || bucket == "" {
		return nil
	}
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
	client := s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String(endpoint),
		Credentials:  credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
	})
	return &R2Client{
		presign: s3.NewPresignClient(client),
		client:  client,
		bucket:  bucket,
	}
}

// PresignPutObject は指定キーへの署名付きPUT URL（15分有効）を発行する。
func (r *R2Client) PresignPutObject(ctx context.Context, key, contentType string) (string, error) {
	out, err := r.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(15*time.Minute))
	if err != nil {
		return "", err
	}
	return out.URL, nil
}

// DeleteObject はR2上のオブジェクトを削除する。
func (r *R2Client) DeleteObject(ctx context.Context, key string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	return err
}
