package fyne

import "strings"

// versionTrackHintText returns the per-client note about what "latest" resolves
// to, or "" for nightly/custom. JGRPP "latest" is stable-only by design.
func versionTrackHintText(clientID string) string {
	switch clientID {
	case "vanilla":
		return "\"latest (Stable)\" tracks final releases only.\n" +
			"\"latest (Testing)\" also includes betas and release candidates."
	case "jgrpp":
		return "\"latest\" installs the newest stable release.\n" +
			"To play a pre-release, manually pick the version."
	default:
		return ""
	}
}

// defaultVersionOptions returns the version dropdown presets for a client track.
func defaultVersionOptions(clientID string) []string {
	switch clientID {
	case "vanilla":
		return []string{"latest (Stable)", "latest (Testing)"}
	case "vanilla-nightly":
		return []string{"latest"}
	default:
		return []string{"latest"}
	}
}

// versionOptionsFor assembles the version dropdown: the per-track "latest"
// presets first, then the fetched list, which is pure network data.
func versionOptionsFor(clientID string, fetched []string) []string {
	return append(defaultVersionOptions(clientID), fetched...)
}

// displayVersion turns a stored version into the field's display string, normalizing the "latest" aliases per track.
func displayVersion(clientID, stored string) string {
	s := strings.TrimSpace(stored)
	lower := strings.ToLower(s)
	switch clientID {
	case "vanilla", "vanilla-nightly":
		if clientID == "vanilla-nightly" {
			switch lower {
			case "", "latest", "latest-stable", "latest-testing", "latest (stable)", "latest (testing)":
				return "latest"
			default:
				return s
			}
		}
		switch lower {
		case "", "latest", "latest-stable", "latest (stable)":
			return "latest (Stable)"
		case "latest-testing", "latest (testing)":
			return "latest (Testing)"
		default:
			return s
		}
	default:
		if s == "" {
			return "latest"
		}
		return s
	}
}

// storedVersion is the inverse of displayVersion: field text to the canonical value persisted on the profile.
func storedVersion(clientID, entered string) string {
	s := strings.TrimSpace(entered)
	lower := strings.ToLower(s)
	switch clientID {
	case "vanilla", "vanilla-nightly":
		if clientID == "vanilla-nightly" {
			if lower == "" || lower == "latest" || lower == "latest-stable" || lower == "latest-testing" || lower == "latest (stable)" || lower == "latest (testing)" {
				return ""
			}
			return s
		}
		switch lower {
		case "", "latest", "latest-stable", "latest (stable)":
			return "latest-stable"
		case "latest-testing", "latest (testing)":
			return "latest-testing"
		default:
			return s
		}
	default:
		if lower == "" || lower == "latest" {
			return ""
		}
		return s
	}
}

// Empty selection means jgrpp (save-handler default), not Config.DefaultClient: don't route this through EffectiveClient.
func companyPasswordClientID(cli string) string {
	if cli == "" {
		return "jgrpp"
	}
	return cli
}
