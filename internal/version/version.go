// Package version formats flakylint build metadata for the version command.
package version

import (
	"fmt"
	"runtime/debug"
)

// Format renders the version line from ldflag-injected build metadata,
// falling back to runtime/debug.ReadBuildInfo when the values are empty
// (for example a plain `go install` build carries no -X ldflags).
func Format(version, commit, date string) string {
	info, ok := debug.ReadBuildInfo()
	return format(version, commit, date, info, ok)
}

func format(version, commit, date string, info *debug.BuildInfo, ok bool) string {
	if version == "" {
		version = "devel"
		if ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
	if commit == "" {
		commit = "none"
		if ok {
			if rev := setting(info, "vcs.revision"); rev != "" {
				commit = rev
			}
		}
	}
	if date == "" {
		date = "unknown"
		if ok {
			if t := setting(info, "vcs.time"); t != "" {
				date = t
			}
		}
	}
	return fmt.Sprintf("flakylint %s (commit %s, built %s)", version, commit, date)
}

func setting(info *debug.BuildInfo, key string) string {
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}
