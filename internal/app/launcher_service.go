package app

import (
	"context"
	"fmt"

	"runttd/internal/domain"
	"runttd/internal/platform"
)

// LauncherService coordinates check, download, extraction and execution for the OpenTTD packages
type LauncherService struct{}

// NewLauncherService constructs a new LauncherService instance
func NewLauncherService() *LauncherService {
	return &LauncherService{}
}

// ResolveVersionFolder searches for the folder of a profile version, querying the network for 'latest' tags if required
func (s *LauncherService) ResolveVersionFolder(ctx context.Context, profile domain.Profile, cfg *domain.Config) (string, error) {
	client := profile.Client
	if client == "" {
		client = "jgrpp" // default
	}

	version := profile.Version
	if version == "" || version == "latest" {
		localLatest := platform.FindLatestFolderClientWithConfig(platform.ClientDownloadDir(cfg, client), client, cfg)
		if localLatest != "" {
			return localLatest, nil
		}

		latestTag, err := ClientLatest(ctx, client, cfg)
		if err != nil || latestTag == "" {
			return "", fmt.Errorf("could not discover latest version tag: %w", err)
		}
		version = latestTag
	}

	folder, err := ClientFindInstalled(ctx, client, version, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to check local installation: %w", err)
	}
	if folder != "" {
		return folder, nil
	}

	return "", fmt.Errorf("version folder not found locally for version: %s (client: %s)", version, client)
}

// EnsureInstalled verifies if a version folder exists locally; if not, triggers the download and extraction pipeline
func (s *LauncherService) EnsureInstalled(ctx context.Context, profile domain.Profile, cfg *domain.Config, logger *platform.Logger) (string, error) {
	client := profile.Client
	if client == "" {
		client = "jgrpp"
	}

	version := profile.Version
	if version == "" || version == "latest" {
		latestTag, err := ClientLatest(ctx, client, cfg)
		if err != nil || latestTag == "" {
			return "", fmt.Errorf("could not discover latest version tag: %w", err)
		}
		version = latestTag
	}

	folder, err := ClientFindInstalled(ctx, client, version, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to check local installation: %w", err)
	}
	if folder != "" {
		return folder, nil
	}

	if logger != nil {
		logger.Append(fmt.Sprintf("Downloading %s version %s...", client, version))
	}

	ok, err := ClientDownloadAndExtract(ctx, client, version, cfg, logger)
	if err != nil || !ok {
		return "", fmt.Errorf("download/extraction failed for version %s: %w", version, err)
	}

	folder, err = ClientFindInstalled(ctx, client, version, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to verify installation: %w", err)
	}
	if folder == "" {
		return "", fmt.Errorf("installation verification failed for version %s", version)
	}

	if logger != nil {
		logger.Append(fmt.Sprintf("Installation of version %s successful", version))
	}
	return folder, nil
}
