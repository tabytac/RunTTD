package domain

// ReleaseAsset represents a single downloadable artifact associated with a GitHub release
type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// ReleaseInfo contains GitHub release tags and associated download assets
type ReleaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []ReleaseAsset `json:"assets"`
}

// NightlyManifestData encapsulates the list of file identifiers returned by CDN manifests
type NightlyManifestData struct {
	FileIDs []string
}
