package platform

import (
	"strconv"
	"strings"
)

// VanillaNeedsBaseSetWarning reports whether a vanilla version predates the
// bundled free graphics first shipped in 1.2.0-beta1. Pre-release suffixes
// share the same numeric (major, minor) as the release, so they're handled
// correctly by stripping before parsing. Unparseable inputs return false.
func VanillaNeedsBaseSetWarning(version string) bool {
	v := strings.TrimSpace(version)
	if dash := strings.IndexByte(v, '-'); dash >= 0 {
		v = v[:dash]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return major < 1 || (major == 1 && minor < 2)
}
