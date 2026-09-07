package main

import (
	"github.com/giantswarm/mcp-kubernetes/cmd"
	"github.com/giantswarm/mcp-kubernetes/pkg/project"
)

func main() {
	// The version comes from pkg/project: the architect CI and the devctl
	// Makefile stamp it through -ldflags -X at link time; a plain `go build`
	// falls back to Go's VCS build info. Nothing sets a variable in main any
	// more (the goreleaser hook that once did is long gone, so release
	// binaries printed "dev").
	cmd.SetVersion(project.Version())

	// Execute the root command
	cmd.Execute()
}
