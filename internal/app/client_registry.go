package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

var (
	registryMu     sync.RWMutex
	clientRegistry = map[string]Client{}

	// ErrUnknownClient is returned when a client ID has not been registered.
	ErrUnknownClient = errors.New("unknown client ID")
)

// RegisterClient maps a Client identifier to its implementation instance.
// This is thread-safe and can be called from main initialization or test mocks.
func RegisterClient(c Client) {
	registryMu.Lock()
	defer registryMu.Unlock()
	clientRegistry[c.ID()] = c
}

// RegisterBuiltInClients registers the standard built-in engine tracks (JGRPP, Stable Vanilla, Nightly Vanilla, and Custom)
func RegisterBuiltInClients() {
	RegisterClient(&jgrppClient{})
	RegisterClient(&vanillaClient{nightly: false})
	RegisterClient(&vanillaClient{nightly: true})
	RegisterClient(&customClient{})
}

// GetClient retrieves the Client implementation mapped to the specified identifier.
// Returns nil if the client is not registered.
func GetClient(id string) Client {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return clientRegistry[id]
}

// jgrppClient wraps platform GitHub releases helpers
type jgrppClient struct{}

func (j *jgrppClient) ID() string          { return "jgrpp" }
func (j *jgrppClient) DisplayName() string { return "JGRPP" }
func (j *jgrppClient) FetchVersions(ctx context.Context, cfg *domain.Config) ([]string, error) {
	return platform.FetchAvailableVersions(ctx, cfg)
}
func (j *jgrppClient) Latest(ctx context.Context, cfg *domain.Config) (string, error) {
	return platform.CheckForNewVersion(ctx, cfg), nil
}
func (j *jgrppClient) DownloadAndExtract(ctx context.Context, version string, cfg *domain.Config) (bool, error) {
	return platform.DownloadAndExtractVersion(ctx, version, cfg), nil
}
func (j *jgrppClient) FindInstalled(ctx context.Context, version string, cfg *domain.Config) (string, error) {
	folder := platform.FindVersionFolderClient(platform.ClientDownloadDir(cfg, "jgrpp"), version, "jgrpp", cfg)
	return folder, nil
}

// customClient is a placeholder for user-managed executables. All "what version is available
// remotely" and "download/extract" methods are no-ops; the profile carries the folder directly.
type customClient struct{}

func (c *customClient) ID() string          { return "custom" }
func (c *customClient) DisplayName() string { return "Custom Executable" }
func (c *customClient) FetchVersions(ctx context.Context, cfg *domain.Config) ([]string, error) {
	return nil, nil
}
func (c *customClient) Latest(ctx context.Context, cfg *domain.Config) (string, error) {
	return "", nil
}
func (c *customClient) DownloadAndExtract(ctx context.Context, version string, cfg *domain.Config) (bool, error) {
	return false, nil
}
func (c *customClient) FindInstalled(ctx context.Context, version string, cfg *domain.Config) (string, error) {
	return "", nil
}

// vanillaClient uses CDN mirrors
type vanillaClient struct{ nightly bool }

func (v *vanillaClient) ID() string {
	if v.nightly {
		return "vanilla-nightly"
	}
	return "vanilla"
}
func (v *vanillaClient) DisplayName() string {
	if v.nightly {
		return "OpenTTD (Nightly)"
	}
	return "OpenTTD (Stable)"
}
func (v *vanillaClient) FetchVersions(ctx context.Context, cfg *domain.Config) ([]string, error) {
	return platform.FetchAvailableVersionsForClient(ctx, v.ID(), cfg)
}
func (v *vanillaClient) Latest(ctx context.Context, cfg *domain.Config) (string, error) {
	latest := platform.CheckForNewVersionForClient(ctx, v.ID(), cfg)
	return latest, nil
}
func (v *vanillaClient) DownloadAndExtract(ctx context.Context, version string, cfg *domain.Config) (bool, error) {
	ok := platform.DownloadAndExtractVersionForClient(ctx, version, v.ID(), cfg)
	return ok, nil
}
func (v *vanillaClient) FindInstalled(ctx context.Context, version string, cfg *domain.Config) (string, error) {
	folder := platform.FindVersionFolderClient(platform.ClientDownloadDir(cfg, v.ID()), version, v.ID(), cfg)
	return folder, nil
}

// Convenience wrappers that query the thread-safe registry with context.

func ClientFetchVersions(ctx context.Context, clientID string, cfg *domain.Config) ([]string, error) {
	c := GetClient(clientID)
	if c == nil {
		return nil, fmt.Errorf("%w: %s", ErrUnknownClient, clientID)
	}
	return c.FetchVersions(ctx, cfg)
}

func ClientLatest(ctx context.Context, clientID string, cfg *domain.Config) (string, error) {
	c := GetClient(clientID)
	if c == nil {
		return "", fmt.Errorf("%w: %s", ErrUnknownClient, clientID)
	}
	return c.Latest(ctx, cfg)
}

func ClientDownloadAndExtract(ctx context.Context, clientID, version string, cfg *domain.Config) (bool, error) {
	c := GetClient(clientID)
	if c == nil {
		return false, fmt.Errorf("%w: %s", ErrUnknownClient, clientID)
	}
	return c.DownloadAndExtract(ctx, version, cfg)
}

func ClientFindInstalled(ctx context.Context, clientID, version string, cfg *domain.Config) (string, error) {
	c := GetClient(clientID)
	if c == nil {
		return "", fmt.Errorf("%w: %s", ErrUnknownClient, clientID)
	}
	return c.FindInstalled(ctx, version, cfg)
}
