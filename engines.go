package main

// Engine defines a pluggable engine adapter
type Engine interface {
	ID() string
	DisplayName() string
	FetchVersions(cfg *Config) ([]string, error)
	Latest(cfg *Config) (string, error)
	DownloadAndExtract(version string, cfg *Config) (bool, error)
	FindInstalled(parentDir, version string, cfg *Config) string
}

var engineRegistry = map[string]Engine{}

func RegisterEngine(e Engine) {
	engineRegistry[e.ID()] = e
}

func GetEngine(id string) Engine {
	return engineRegistry[id]
}

// jgrppEngine wraps existing GitHub-based helpers
type jgrppEngine struct{}

func (j *jgrppEngine) ID() string          { return "jgrpp" }
func (j *jgrppEngine) DisplayName() string { return "JGRPP" }
func (j *jgrppEngine) FetchVersions(cfg *Config) ([]string, error) {
	return FetchAvailableVersions(cfg)
}
func (j *jgrppEngine) Latest(cfg *Config) (string, error) { return CheckForNewVersion(cfg), nil }
func (j *jgrppEngine) DownloadAndExtract(version string, cfg *Config) (bool, error) {
	return DownloadAndExtractVersion(version, cfg), nil
}
func (j *jgrppEngine) FindInstalled(parentDir, version string, cfg *Config) string {
	return FindVersionFolderEngine(parentDir, version, "jgrpp", cfg)
}

// vanillaEngine uses CDN mirrors
type vanillaEngine struct{ nightly bool }

func (v *vanillaEngine) ID() string {
	if v.nightly {
		return "vanilla-nightly"
	}
	return "vanilla"
}
func (v *vanillaEngine) DisplayName() string {
	if v.nightly {
		return "OpenTTD (Nightly)"
	}
	return "OpenTTD (Stable)"
}
func (v *vanillaEngine) FetchVersions(cfg *Config) ([]string, error) {
	return FetchAvailableVersionsForEngine(v.ID(), cfg)
}
func (v *vanillaEngine) Latest(cfg *Config) (string, error) {
	latest := CheckForNewVersionForEngine(v.ID(), cfg)
	return latest, nil
}
func (v *vanillaEngine) DownloadAndExtract(version string, cfg *Config) (bool, error) {
	ok := DownloadAndExtractVersionForEngine(version, v.ID(), cfg)
	return ok, nil
}
func (v *vanillaEngine) FindInstalled(parentDir, version string, cfg *Config) string {
	return FindVersionFolderEngine(parentDir, version, v.ID(), cfg)
}


// initialize built-in engines
func init() {
	RegisterEngine(&jgrppEngine{})
	RegisterEngine(&vanillaEngine{nightly: false})
	RegisterEngine(&vanillaEngine{nightly: true})
}

// Convenience wrappers that prefer registered engines but fall back to legacy helpers.
func EngineFetchVersions(engineID string, cfg *Config) ([]string, error) {
	if e := GetEngine(engineID); e != nil {
		return e.FetchVersions(cfg)
	}
	return FetchAvailableVersionsForEngine(engineID, cfg)
}

func EngineLatest(engineID string, cfg *Config) (string, error) {
	if e := GetEngine(engineID); e != nil {
		return e.Latest(cfg)
	}
	return CheckForNewVersionForEngine(engineID, cfg), nil
}

func EngineDownloadAndExtract(engineID, version string, cfg *Config) (bool, error) {
	if e := GetEngine(engineID); e != nil {
		ok, err := e.DownloadAndExtract(version, cfg)
		return ok, err
	}
	ok := DownloadAndExtractVersionForEngine(version, engineID, cfg)
	return ok, nil
}

func EngineFindInstalled(engineID, parentDir, version string, cfg *Config) string {
	if e := GetEngine(engineID); e != nil {
		return e.FindInstalled(parentDir, version, cfg)
	}
	return FindVersionFolderEngine(parentDir, version, engineID, cfg)
}
