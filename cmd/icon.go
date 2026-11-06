package cmd

import _ "embed"

// kubernetesLogo is the Kubernetes logo the server advertises as its MCP icon.
// Source: github.com/cncf/artwork projects/kubernetes/icon/color/kubernetes-icon-color.svg
// at 40e2e8948509b40e4bad479446aaec18d6273bf2, minified losslessly with styles inlined.
//
//go:embed kubernetes.svg
var kubernetesLogo []byte
