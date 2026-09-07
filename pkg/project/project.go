package project

import "runtime/debug"

// dev is the default for unset build identifiers. Local `go build`
// invocations without ldflags keep this so `mcp-kubernetes --version` stays printable.
const dev = "dev"

// devel is the module version runtime/debug reports for a build that carries
// no resolvable VCS tag (no .git, or built outside a module checkout).
const devel = "(devel)"

// Build identifiers, stamped at link time via `-X` ldflags: the architect-orb
// `go-build` job sets `gitSHA` (from `CIRCLE_SHA1`), `buildTimestamp` (UTC
// build time) and, on tag builds since architect-orb#924, `version` (the tag
// without its v); the devctl Makefile stamps the same three locally. Without
// ldflags, Version falls back to the Go build info, which the toolchain
// stamps from the VCS tag (a clean semver only when the checkout sits exactly
// on a tag of the module's major, `+dirty` with any untracked file).
var (
	version        = dev
	gitSHA         = dev
	buildTimestamp = "unknown"
)

// Version returns the best human-readable build identifier available, in
// order: an explicitly injected `version` ldflag, the VCS version stamped into
// the Go build info, the injected commit SHA, and finally the placeholder
// "dev".
func Version() string {
	if version != dev && version != "" {
		return version
	}
	if v := buildInfoVersion(); v != "" {
		return v
	}
	if gitSHA != dev {
		return gitSHA
	}
	return dev
}

// buildInfoVersion reads the main module version the Go toolchain embedded from
// version control. It returns "" when no usable version is present — either no
// build info, or the "(devel)" placeholder a tag-less build produces — so
// Version can fall through to the next source.
var buildInfoVersion = func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	if v := info.Main.Version; v != "" && v != devel {
		return v
	}
	return ""
}

// GitSHA returns the commit SHA the binary was built from.
func GitSHA() string { return gitSHA }

// BuildTimestamp returns the UTC build time in RFC 3339 format, or
// "unknown" when no ldflag was injected.
func BuildTimestamp() string { return buildTimestamp }
