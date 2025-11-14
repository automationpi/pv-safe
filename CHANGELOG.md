# Changelog

All notable changes to pv-safe will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial release of pv-safe
- Kubernetes admission webhook for PersistentVolume deletion protection
- Risk assessment based on PV reclaim policies
- VolumeSnapshot support for safe deletions
- Label-based bypass mechanism
- Helm chart for easy installation
- Comprehensive operator documentation
- Multi-architecture container images (amd64, arm64)

### Features
- Automatic blocking of risky PV/PVC/Namespace deletions
- Integration with cert-manager for TLS certificates
- Detailed error messages with remediation steps
- Audit logging for all operations
- High availability with multiple replicas
- Graceful degradation without VolumeSnapshot CRDs

## [0.1.0] - 2025-11-14

### Added
- Initial development release
- Core webhook functionality
- Basic risk assessment
- Documentation

[Unreleased]: https://github.com/automationpi/pv-safe/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/automationpi/pv-safe/releases/tag/v0.1.0
