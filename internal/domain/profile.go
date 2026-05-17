package domain

// Profile represents a launch profile configured by the user
type Profile struct {
	Name                  string `json:"name"`
	Version               string `json:"version"`
	SavePath              string `json:"savePath"`
	ConfigFilePath        string `json:"configFilePath"`
	NoConfigSave          bool   `json:"noConfigSave"`
	ServerIpPort          string `json:"serverIpPort"`
	ServerPassword        string `json:"serverPassword"`
	ServerCompanyNumber   string `json:"serverCompanyNumber"`
	ServerCompanyPassword string `json:"serverCompanyPassword"`
	LaunchMode            string `json:"launchMode"` // "", "file", "folder", "multiplayer"
	AutoLatestFilter      string `json:"autoLatestFilter"`
	ExtraArgs             string `json:"extraArgs"`
	Client                string `json:"client"` // "jgrpp", "vanilla", "vanilla-nightly"
}
