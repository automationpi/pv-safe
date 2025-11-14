# Contributing to pv-safe

Thank you for your interest in contributing to pv-safe! This document provides guidelines and instructions for contributing.

## Code of Conduct

We are committed to providing a welcoming and inclusive environment. Please be respectful and constructive in all interactions.

## Getting Started

### Prerequisites

- Go 1.21+
- Docker
- kind (Kubernetes in Docker)
- kubectl
- make

### Development Setup

1. **Fork and clone the repository:**
```bash
git clone https://github.com/automationpi/pv-safe.git
cd pv-safe
```

2. **Set up development environment:**
```bash
# Install dependencies
go mod download

# Create a test cluster
make setup
```

3. **Build and test:**
```bash
# Build the webhook
make webhook-build

# Deploy to test cluster
make webhook-deploy

# View logs
make webhook-logs
```

## Development Workflow

### Making Changes

1. **Create a feature branch:**
```bash
git checkout -b feature/your-feature-name
```

2. **Make your changes:**
   - Write clear, idiomatic Go code
   - Follow existing code style and patterns
   - Add comments for complex logic
   - Update documentation as needed

3. **Test your changes:**
```bash
# Run unit tests
go test ./...

# Test in local cluster
make webhook-build
make webhook-deploy

# Test various scenarios
kubectl delete pvc <test-pvc> -n <test-namespace>
```

4. **Commit your changes:**
```bash
git add .
git commit -m "feat: add new feature"
```

### Commit Message Guidelines

We follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation changes
- `refactor:` Code refactoring
- `test:` Test additions or changes
- `chore:` Build process or tooling changes

Examples:
```
feat: add support for StatefulSet PVC protection
fix: correct snapshot detection for cross-namespace refs
docs: update operator guide with new examples
refactor: simplify risk assessment logic
test: add integration tests for namespace deletion
```

## Code Guidelines

### Go Code Style

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` to format code
- Run `go vet` to catch common issues
- Keep functions focused and testable
- Add godoc comments for exported functions

Example:
```go
// AssessPVCDeletion checks if deleting a PVC would lose data.
// It examines the PV reclaim policy and checks for VolumeSnapshots.
// Returns a RiskAssessment indicating whether deletion is safe.
func (rc *RiskCalculator) AssessPVCDeletion(ctx context.Context, namespace, name string) (*RiskAssessment, error) {
    // Implementation
}
```

### Error Handling

- Always check and handle errors
- Provide context in error messages
- Use `fmt.Errorf` with `%w` for wrapping errors

```go
pvc, err := rc.client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
if err != nil {
    return nil, fmt.Errorf("failed to get PVC %s/%s: %w", namespace, name, err)
}
```

### Logging

- Use structured logging with clear context
- Include resource names and namespaces
- Log at appropriate levels (info, warning, error)

```go
h.Logger.Printf("BLOCKING: Risky deletion detected!")
h.Logger.Printf("  Resource: %s/%s", namespace, name)
h.Logger.Printf("  Reason: %s", assessment.Message)
```

## Testing

### Unit Tests

Add unit tests for new functionality:

```go
func TestAssessPVCDeletion_WithRetainPolicy(t *testing.T) {
    // Test setup
    // Test execution
    // Assertions
}
```

### Integration Tests

Test end-to-end scenarios in a local cluster:

1. Deploy webhook
2. Create test resources
3. Attempt deletion
4. Verify expected behavior

### Test Coverage

- Aim for >80% code coverage
- Test both success and failure paths
- Test edge cases

## Documentation

### Code Documentation

- Add godoc comments for all exported types and functions
- Include usage examples in comments
- Document complex algorithms

### User Documentation

Update relevant documentation:
- `README.md` - For user-facing changes
- `docs/ARCHITECTURE.md` - For design or architectural changes
- `docs/DEVELOPMENT.md` - For development workflow changes
- `docs/TROUBLESHOOTING.md` - For common issues and solutions

### Examples

When adding new features, include:
- Usage examples in README
- Test fixtures in `test/fixtures/`
- Step-by-step guides in operator docs

## Pull Request Process

### Before Submitting

1. **Ensure tests pass:**
```bash
go test ./...
make test
```

2. **Update documentation:**
   - Update README if needed
   - Update operator guide for new features
   - Add examples

3. **Check code quality:**
```bash
gofmt -w .
go vet ./...

# Optional: Run golangci-lint locally
golangci-lint run
```

4. **Verify builds:**
```bash
# Test Docker build
docker build -t pv-safe-test .

# Test Helm chart
helm lint charts/pv-safe
helm template pv-safe charts/pv-safe --debug
```

### Automated Checks

When you create a PR, the following workflows will run automatically:

- **CI Workflow**: Linting, tests, security scans
- **Build Workflow**: Docker image build (multi-arch)
- **Helm Lint**: Chart validation

All checks must pass before merge.

### Submitting PR

1. **Push your branch:**
```bash
git push origin feature/your-feature-name
```

2. **Create Pull Request:**
   - Use a clear, descriptive title
   - Reference related issues
   - Describe changes and motivation
   - Include test results
   - Add screenshots/examples if applicable

3. **PR Template:**
```markdown
## Description
Brief description of changes

## Motivation
Why is this change needed?

## Changes
- List of specific changes
- Impact on existing functionality

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests performed
- [ ] Documentation updated

## Checklist
- [ ] Code follows project style
- [ ] Tests pass locally
- [ ] Documentation updated
- [ ] Commit messages follow guidelines
```

### Review Process

- Maintainers will review your PR
- Address feedback and comments
- Make requested changes
- Once approved, PR will be merged

## Reporting Issues

### Bug Reports

Include:
- pv-safe version
- Kubernetes version
- Description of the issue
- Steps to reproduce
- Expected vs actual behavior
- Logs and error messages

### Feature Requests

Include:
- Use case description
- Proposed solution
- Alternatives considered
- Additional context

## Project Structure

Understanding the codebase:

```
pv-safe/
├── cmd/webhook/main.go              # Webhook entry point
├── internal/webhook/
│   ├── handler.go                   # HTTP handler and admission logic
│   ├── risk.go                      # Risk assessment engine
│   ├── snapshot.go                  # VolumeSnapshot detection
│   └── client.go                    # Kubernetes client setup
├── deploy/                          # Kubernetes manifests
├── docs/                            # Documentation
├── scripts/                         # Build and deployment scripts
└── test/fixtures/                   # Test scenarios
```

### Key Components

- **Handler**: Processes admission requests, implements http.Handler
- **RiskCalculator**: Analyzes PV policies and snapshots
- **SnapshotChecker**: Interacts with VolumeSnapshot API
- **Deployment**: Kubernetes manifests for installation

## Getting Help

- **Documentation**: Check [docs/](docs/) first
- **Issues**: Search existing issues
- **Discussions**: Ask questions in GitHub Discussions
- **Slack**: Join our community Slack (link TBD)

## Recognition

Contributors will be:
- Listed in release notes
- Added to CONTRIBUTORS file
- Acknowledged in project documentation

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

---

Thank you for contributing to pv-safe! Your efforts help make Kubernetes storage operations safer for everyone.
