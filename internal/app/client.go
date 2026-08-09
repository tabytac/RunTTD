package app

import (
	"context"

	"runttd/internal/domain"
)

// Client is the registry contract for an engine track: its identity and
// display name, its selectable version list, and where a version is installed.
// The launch pipeline is deliberately not behind it; launch.go dispatches on
// client ID directly, since resolving and downloading need track, logger and
// progress plumbing plus per-client policy this interface does not carry.
type Client interface {
	ID() string
	DisplayName() string
	FetchVersions(ctx context.Context, cfg *domain.Config) ([]string, error)
	FindInstalled(ctx context.Context, version string, cfg *domain.Config) (string, error)
}
