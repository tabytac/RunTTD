package main

// Client defines a pluggable client/variant adapter
type Client interface {
	ID() string
	DisplayName() string
	FetchVersions(cfg *Config) ([]string, error)
	Latest(cfg *Config) (string, error)
	DownloadAndExtract(version string, cfg *Config) (bool, error)
	FindInstalled(parentDir, version string, cfg *Config) string
}

var clientRegistry = map[string]Client{}

func RegisterClient(c Client) {
	clientRegistry[c.ID()] = c
}

func GetClient(id string) Client {
	return clientRegistry[id]
}

// jgrppClient wraps existing GitHub-based helpers
type jgrppClient struct{}

func (j *jgrppClient) ID() string          { return "jgrpp" }
func (j *jgrppClient) DisplayName() string { return "JGRPP" }
func (j *jgrppClient) FetchVersions(cfg *Config) ([]string, error) {
	return FetchAvailableVersions(cfg)
}
func (j *jgrppClient) Latest(cfg *Config) (string, error) { return CheckForNewVersion(cfg), nil }
func (j *jgrppClient) DownloadAndExtract(version string, cfg *Config) (bool, error) {
	return DownloadAndExtractVersion(version, cfg), nil
}
func (j *jgrppClient) FindInstalled(parentDir, version string, cfg *Config) string {
	return FindVersionFolderClient(parentDir, version, "jgrpp", cfg)
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
func (v *vanillaClient) FetchVersions(cfg *Config) ([]string, error) {
	return FetchAvailableVersionsForClient(v.ID(), cfg)
}
func (v *vanillaClient) Latest(cfg *Config) (string, error) {
	latest := CheckForNewVersionForClient(v.ID(), cfg)
	return latest, nil
}
func (v *vanillaClient) DownloadAndExtract(version string, cfg *Config) (bool, error) {
	ok := DownloadAndExtractVersionForClient(version, v.ID(), cfg)
	return ok, nil
}
func (v *vanillaClient) FindInstalled(parentDir, version string, cfg *Config) string {
	return FindVersionFolderClient(parentDir, version, v.ID(), cfg)
}

// initialize built-in clients
func init() {
	RegisterClient(&jgrppClient{})
	RegisterClient(&vanillaClient{nightly: false})
	RegisterClient(&vanillaClient{nightly: true})
}

// Convenience wrappers that prefer registered clients but fall back to legacy helpers.
func ClientFetchVersions(clientID string, cfg *Config) ([]string, error) {
	if c := GetClient(clientID); c != nil {
		return c.FetchVersions(cfg)
	}
	return FetchAvailableVersionsForClient(clientID, cfg)
}

func ClientLatest(clientID string, cfg *Config) (string, error) {
	if c := GetClient(clientID); c != nil {
		return c.Latest(cfg)
	}
	return CheckForNewVersionForClient(clientID, cfg), nil
}

func ClientDownloadAndExtract(clientID, version string, cfg *Config) (bool, error) {
	if c := GetClient(clientID); c != nil {
		ok, err := c.DownloadAndExtract(version, cfg)
		return ok, err
	}
	ok := DownloadAndExtractVersionForClient(version, clientID, cfg)
	return ok, nil
}

func ClientFindInstalled(clientID, parentDir, version string, cfg *Config) string {
	if c := GetClient(clientID); c != nil {
		return c.FindInstalled(parentDir, version, cfg)
	}
	return FindVersionFolderClient(parentDir, version, clientID, cfg)
}
