# GitHub Actions Workflows

This directory contains CI/CD workflows for pv-safe.

## Workflows

### 1. CI (ci.yaml)

**Triggers:** Push to main/master, Pull Requests

**Purpose:** Continuous integration checks

**Jobs:**
- **Lint**: Go code linting with golangci-lint
- **Test**: Run unit tests with coverage
- **Helm Lint**: Validate Helm chart syntax
- **Docker Build**: Test Docker image build
- **Security**: Trivy vulnerability scanning

### 2. Build and Push Image (build.yaml)

**Triggers:** Push to main/master, Tags, Pull Requests

**Purpose:** Build and publish container images

**Jobs:**
- Builds multi-arch images (amd64, arm64)
- Pushes to GitHub Container Registry (ghcr.io)
- Generates attestations
- Tags images with version, branch, SHA

**Image Tags Generated:**
- `ghcr.io/automationpi/pv-safe:latest` (main branch)
- `ghcr.io/automationpi/pv-safe:v1.0.0` (version tags)
- `ghcr.io/automationpi/pv-safe:v1.0` (major.minor)
- `ghcr.io/automationpi/pv-safe:v1` (major)
- `ghcr.io/automationpi/pv-safe:main-abc1234` (branch-sha)

### 3. Release (release.yaml)

**Triggers:** Push tags matching `v*`

**Purpose:** Create GitHub releases with artifacts

**Jobs:**
- Runs tests
- Builds binaries for multiple platforms
- Generates SBOM (Software Bill of Materials)
- Packages Helm chart
- Creates GitHub release with:
  - Release notes
  - Binaries (linux/darwin, amd64/arm64)
  - Helm chart package
  - SBOM file
- Publishes Helm chart to OCI registry

### 4. Publish Helm Chart (chart-publish.yaml)

**Triggers:** Push to main/master (when charts/ changes), Manual

**Purpose:** Publish Helm chart

**Jobs:**
- Packages Helm chart
- Pushes to OCI registry (ghcr.io)
- Updates GitHub Pages with chart index

## Container Registry

Images are published to GitHub Container Registry:

```bash
# Pull latest
docker pull ghcr.io/automationpi/pv-safe:latest

# Pull specific version
docker pull ghcr.io/automationpi/pv-safe:v0.1.0

# Multi-arch support (automatic)
docker pull ghcr.io/automationpi/pv-safe:latest # pulls correct arch
```

## Helm Chart Repository

Helm charts are available as OCI artifacts:

```bash
# Install from OCI registry
helm install pv-safe oci://ghcr.io/automationpi/pv-safe --version 0.1.0

# Or from GitHub Pages
helm repo add pv-safe https://automationpi.github.io/pv-safe
helm repo update
helm install pv-safe pv-safe/pv-safe
```

## Release Process

To create a new release:

1. **Update version numbers:**
   - `charts/pv-safe/Chart.yaml` (version and appVersion)
   - `CHANGELOG.md` (add new version section)

2. **Commit changes:**
   ```bash
   git add .
   git commit -m "chore: prepare release v0.2.0"
   git push
   ```

3. **Create and push tag:**
   ```bash
   git tag -a v0.2.0 -m "Release v0.2.0"
   git push origin v0.2.0
   ```

4. **Automated actions:**
   - CI workflow runs tests
   - Build workflow builds multi-arch images
   - Release workflow:
     - Builds binaries
     - Creates GitHub release
     - Publishes Helm chart

5. **Verify release:**
   - Check GitHub releases page
   - Verify container images on ghcr.io
   - Test Helm chart installation

## Secrets Required

The workflows use the following secrets (automatically available):

- `GITHUB_TOKEN` - Provided by GitHub Actions
  - Used for: Package registry, creating releases, code scanning

## Permissions

Workflows require these permissions:

- `contents: write` - Create releases, update pages
- `packages: write` - Push to container registry
- `security-events: write` - Upload security scan results
- `id-token: write` - Generate attestations

## Troubleshooting

### Build Fails

Check the build logs in Actions tab. Common issues:
- Go compilation errors
- Linting failures
- Test failures

### Image Push Fails

Ensure:
- Repository has packages enabled
- Workflow has correct permissions
- Image name matches repository

### Helm Chart Publish Fails

Verify:
- Chart.yaml has correct version
- Chart passes lint checks
- OCI registry is accessible

## Local Testing

Test workflows locally before pushing:

```bash
# Install act (https://github.com/nektos/act)
brew install act  # macOS
# or download from releases

# Test CI workflow
act pull_request -W .github/workflows/ci.yaml

# Test build workflow
act push -W .github/workflows/build.yaml
```

## Security

- All images are scanned with Trivy
- SBOMs are generated for releases
- Build attestations are created
- Images are signed (cosign support planned)

## Monitoring

Monitor workflow runs:
- GitHub Actions tab
- Status badges in README
- Email notifications for failures
