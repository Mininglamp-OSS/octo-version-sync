package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/tencentyun/cos-go-sdk-v5"
)

type COSStore struct {
	client    *cos.Client
	objectKey string
}

func NewCOSStore(bucketURL, secretID, secretKey, folder string) *COSStore {
	u, _ := url.Parse(bucketURL)
	b := &cos.BaseURL{BucketURL: u}
	client := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  secretID,
			SecretKey: secretKey,
		},
	})

	key := "version.json"
	if folder != "" {
		key = folder + "/version.json"
	}

	return &COSStore{
		client:    client,
		objectKey: key,
	}
}

func (s *COSStore) Write(ctx context.Context, data []byte) error {
	_, err := s.client.Object.Put(ctx, s.objectKey, bytes.NewReader(data), &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: "application/json",
		},
	})
	if err != nil {
		return fmt.Errorf("cos put: %w", err)
	}
	return nil
}

func (s *COSStore) Read(ctx context.Context) ([]byte, error) {
	resp, err := s.client.Object.Get(ctx, s.objectKey, nil)
	if err != nil {
		return nil, fmt.Errorf("cos get: %w", err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
