package app

import (
	"context"

	"runttd/internal/domain"
)

// Client defines the contract that different engine types (e.g. Vanilla, JGRPP) must implement to support download, updates, and identification
type Client interface {
	ID() string
	DisplayName() string
	FetchVersions(ctx context.Context, cfg *domain.Config) ([]string, error)
	Latest(ctx context.Context, cfg *domain.Config) (string, error)
	DownloadAndExtract(ctx context.Context, version string, cfg *domain.Config) (bool, error)
	FindInstalled(ctx context.Context, parentDir, version string, cfg *domain.Config) (string, error)
}
