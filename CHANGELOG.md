# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Updated CAPI Cluster discovery to use `cluster.x-k8s.io/v1beta2` API ([#253](https://github.com/giantswarm/mcp-kubernetes/issues/253))
- Switch CI to `push-to-registries-multiarch` with amd64-only on branches
  and full multi-arch on release tags for faster PR builds.
- Run chart tests before pushing to the app catalog.
- Update Dockerfile with multi-stage Go build for buildx multi-arch support.
- Update `mcp-oauth` to v0.2.68.

### Fixed
- Fixed `capi_cluster_health` reporting healthy clusters as UNHEALTHY due to CAPI v1beta2 schema changes ([#287](https://github.com/giantswarm/mcp-kubernetes/issues/287))

## [0.0.19] - 2025-07-24

### Changed
- Align renovate config

## [0.0.18] - 2025-07-24

### Added
- Helm chart for Kubernetes deployment ([#54](https://github.com/giantswarm/mcp-kubernetes/pull/54))
- In-cluster authentication support with `--in-cluster` flag ([#56](https://github.com/giantswarm/mcp-kubernetes/pull/56))
- Support for reading kubeconfig from environment variable ([#58](https://github.com/giantswarm/mcp-kubernetes/pull/58))

### Fixed
- Fixed Go imports formatting ([#51](https://github.com/giantswarm/mcp-kubernetes/pull/51))
- Removed expectation for kubeconfig or context to exist at startup ([#57](https://github.com/giantswarm/mcp-kubernetes/pull/57))

## [0.0.17] - 2025-07-20

### Added
- Comprehensive pagination for MCP tools ([#43](https://github.com/giantswarm/mcp-kubernetes/pull/43))

## [0.0.16] - 2025-07-17

### Fixed
- Updated module github.com/mark3labs/mcp-go to v0.34.0 ([#41](https://github.com/giantswarm/mcp-kubernetes/pull/41))

## [0.0.15] - 2025-07-17

### Fixed
- Updated kubernetes packages to v0.33.3 ([#42](https://github.com/giantswarm/mcp-kubernetes/pull/42))

## [0.0.14] - 2025-07-10

### Fixed
- Relaxed namespace requirement for listing namespaces ([#39](https://github.com/giantswarm/mcp-kubernetes/pull/39))

## [0.0.13] - 2025-07-09

### Fixed
- Updated module github.com/mark3labs/mcp-go to v0.33.0 ([#40](https://github.com/giantswarm/mcp-kubernetes/pull/40))

## [0.0.12] - 2025-07-05

### Added
- Structured response for port forwarding sessions ([#38](https://github.com/giantswarm/mcp-kubernetes/pull/38))

## [0.0.11] - 2025-06-27

### Changed
- Reduced output for resource listing ([#31](https://github.com/giantswarm/mcp-kubernetes/pull/31))

## [0.0.10] - 2025-06-27

### Fixed
- Updated kubernetes packages to v0.33.2 ([#32](https://github.com/giantswarm/mcp-kubernetes/pull/32))

## [0.0.9] - 2025-06-27

### Changed
- Phase 1: Foundation Standardization - Confirm Alignment ([#37](https://github.com/giantswarm/mcp-kubernetes/pull/37))

## [0.0.8] - 2025-06-26

### Removed
- Task-Master ([#30](https://github.com/giantswarm/mcp-kubernetes/pull/30))

## [0.0.7] - 2025-06-26

### Changed
- refactor: remove Helm-related functionality and update documentation by @teemow in #8

## [0.0.6] - 2025-06-26

### Changed
- feat: add version command and improve context tool naming by @teemow in #7

## [0.0.5] - 2025-06-26

### Changed
- Moved main to root folder by @teemow in #6

## [0.0.4] - 2025-06-26

### Changed
- Configure Renovate

## [0.0.3] - 2025-06-26

Initial release

### Added
- Kubernetes client package implementation ([#3](https://github.com/giantswarm/mcp-kubernetes/pull/3))


[Unreleased]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.19...HEAD
[0.0.19]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.18...v0.0.19
[0.0.18]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.17...v0.0.18
[0.0.17]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.16...v0.0.17
[0.0.16]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.15...v0.0.16
[0.0.15]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.14...v0.0.15
[0.0.14]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.13...v0.0.14
[0.0.13]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.12...v0.0.13
[0.0.12]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.11...v0.0.12
[0.0.11]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.10...v0.0.11
[0.0.10]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.9...v0.0.10
[0.0.9]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.8...v0.0.9
[0.0.8]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.7...v0.0.8
[0.0.7]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.6...v0.0.7
[0.0.6]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.5...v0.0.6
[0.0.5]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.4...v0.0.5
[0.0.4]: https://github.com/giantswarm/mcp-kubernetes/compare/v0.0.3...v0.0.4
[0.0.3]: https://github.com/giantswarm/mcp-kubernetes/releases/tag/v0.0.3
