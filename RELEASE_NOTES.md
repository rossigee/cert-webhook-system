## Release Notes

### v0.2.0 - Enhanced Security Scanning & CI/CD (2026-03-06)

#### 🚀 Major Improvements
- **Comprehensive Security Scanning**: Implemented production-grade security scanning using govulncheck, enhanced golangci-lint with security linters (gosec, staticcheck, ineffassign), and Trivy container vulnerability scanning
- **Multi-Platform Docker Builds**: Added cross-compilation support for linux/amd64 and linux/arm64 architectures
- **License Compliance**: Added automatic dependency license checking and SPDX SBOM generation
- **CI/CD Pipeline Overhaul**: Fixed cache corruption issues, added proper permissions, and resolved artifact upload problems

#### 🔧 Technical Enhancements
- Replaced unreliable third-party security tools (OSSF Scorecard) with battle-tested official tools
- Added security-events permissions for SARIF upload to GitHub Security tab
- Implemented proper Docker image tagging and verification
- Enhanced error handling and pipeline reliability

#### 📦 Release Assets
- Multi-platform binaries (Linux amd64/arm64, macOS amd64/arm64, Windows amd64)
- Docker images available at `harbor.golder.lan/library/cert-webhook-system:v0.2.0`
- SPDX SBOM for supply chain transparency
- SHA256 checksums for all assets

#### 🔒 Security Improvements
- Automated vulnerability scanning with govulncheck
- Static security analysis with gosec and other linters  
- Container security scanning with Trivy
- License compliance verification

This release significantly improves the security posture and reliability of the cert-webhook-system, following best practices from major Golang projects like Kubernetes and Docker.