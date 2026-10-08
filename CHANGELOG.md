# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-10-09

### Added

- `stackrun up` starts every service from a `stackrun.yaml` file and streams
  their output with aligned, colour-coded prefixes.
- `stackrun validate` checks a config file and reports every problem at once.
- `stackrun version` prints the build version.
- Strict YAML loading that rejects unknown fields, with service directories
  resolved relative to the config file.
- Graceful shutdown: SIGTERM to each service's process group, a per-service
  `stop_timeout`, then SIGKILL. A second Ctrl+C forces an immediate stop.
- Restart policies (`never`, `on-failure`, `always`) with exponential backoff
  from 1s to 30s, giving up after 5 consecutive restarts.
- Exit codes following Unix conventions: 130 after Ctrl+C, 1 when a service
  fails.

[Unreleased]: https://github.com/rithikamandiv-ux/stackrun/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/rithikamandiv-ux/stackrun/releases/tag/v0.1.0