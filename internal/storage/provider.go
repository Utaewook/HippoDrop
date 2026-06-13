package storage

import "context"

// Provider defines the contract for all cloud storage backends.
type Provider interface {
	Upload(ctx context.Context, localPath, remotePath string) error
	Download(ctx context.Context, remotePath, localPath string) error
	GetPathID(ctx context.Context, path string) (string, error)
}
