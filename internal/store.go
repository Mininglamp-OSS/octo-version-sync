package internal

import "context"

type VersionStore interface {
	Write(ctx context.Context, data []byte) error
	Read(ctx context.Context) ([]byte, error)
}
