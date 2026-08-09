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
	RegisterClient(&vanillaClient{isNightly: false})
	RegisterClient(&vanillaClient{isNightly: true})
	RegisterClient(&customClient{})
}

// GetClient retrieves the Client implementation mapped to the specified identifier.
// Returns nil if the client is not registered.
func GetClient(id string) Client {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return clientRegistry[id]
}

// ClientDisplayName returns the registered display name for a client ID, or ""
// when the ID is unknown, so UI callers choose their own fallback label.
func ClientDisplayName(id string) string {
	if c := GetClient(id); c != nil {
		return c.DisplayName()
	}
	return ""
}

// IsKnownClient reports whether clientID is registered. Empty defaults to jgrpp.
func IsKnownClient(clientID string) bool {
	if clientID == "" {
		clientID = "jgrpp"
	}
	return GetClient(clientID) != nil
}

// jgrppClient wraps platform GitHub releases helpers
type jgrppClient struct{}

func (j *jgrppClient) ID() string          { return "jgrpp" }
func (j *jgrppClient) DisplayName() string { return "JGRPP" }
func (j *jgrppClient) FetchVersions(ctx context.Context, cfg *domain.Config) ([]string, error) {
	return platform.FetchAvailableVersions(ctx, cfg)
}
func (j *jgrppClient) FindInstalled(ctx context.Context, version string, cfg *domain.Config) (string, error) {
	folder := platform.FindVersionFolderClient(platform.ClientDownloadDir(cfg, "jgrpp"), version, "jgrpp", cfg)
	return folder, nil
}

// customClient is a placeholder for user-managed executables: version listing
// and lookup are no-ops, since the profile carries the folder directly.
type customClient struct{}

func (c *customClient) ID() string          { return "custom" }
func (c *customClient) DisplayName() string { return "Custom Executable" }
func (c *customClient) FetchVersions(ctx context.Context, cfg *domain.Config) ([]string, error) {
	return nil, nil
}
func (c *customClient) FindInstalled(ctx context.Context, version string, cfg *domain.Config) (string, error) {
	return "", nil
}

// vanillaClient uses CDN mirrors
type vanillaClient struct{ isNightly bool }

func (v *vanillaClient) ID() string {
	if v.isNightly {
		return "vanilla-nightly"
	}
	return "vanilla"
}
func (v *vanillaClient) DisplayName() string {
	if v.isNightly {
		return "Vanilla OpenTTD (Nightly)"
	}
	return "Vanilla OpenTTD (Releases)"
}
func (v *vanillaClient) FetchVersions(ctx context.Context, cfg *domain.Config) ([]string, error) {
	return platform.FetchAvailableVersionsForClient(ctx, v.ID(), cfg)
}
func (v *vanillaClient) FindInstalled(ctx context.Context, version string, cfg *domain.Config) (string, error) {
	folder := platform.FindVersionFolderClient(platform.ClientDownloadDir(cfg, v.ID()), version, v.ID(), cfg)
	return folder, nil
}

// Convenience wrappers that query the thread-safe registry with context.

// withClient runs fn against the registered clientID, or returns a zero value and wrapped ErrUnknownClient if it is unknown.
func withClient[T any](clientID string, fn func(Client) (T, error)) (T, error) {
	if c := GetClient(clientID); c != nil {
		return fn(c)
	}
	var zero T
	return zero, fmt.Errorf("%w: %s", ErrUnknownClient, clientID)
}

func ClientFetchVersions(ctx context.Context, clientID string, cfg *domain.Config) ([]string, error) {
	return withClient(clientID, func(c Client) ([]string, error) { return c.FetchVersions(ctx, cfg) })
}

func ClientFindInstalled(ctx context.Context, clientID, version string, cfg *domain.Config) (string, error) {
	return withClient(clientID, func(c Client) (string, error) { return c.FindInstalled(ctx, version, cfg) })
}
