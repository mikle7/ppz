// Package version exposes the build-time version + sha to the CLI's
// `ppz version` verb. Both are settable via -ldflags at build time:
//
//	go build -ldflags "-X github.com/pipescloud/ppz/internal/version.Version=v0.3.0 \
//	                   -X github.com/pipescloud/ppz/internal/version.BuildSHA=abc1234" ./cmd/ppz
//
// Defaults are "dev" / "unknown" so a `go build ./...` without ldflags
// still produces a usable binary that self-identifies as a dev build.
package version

var (
	Version  = "dev"
	BuildSHA = "unknown"
)
