# GitHub Actions Workflows

This document describes all GitHub Actions workflows configured for this project.

## Workflows Overview

### 1. CI (Continuous Integration)
**File:** `.github/workflows/ci.yml`  
**Triggers:** Push to `main`/`develop`, Pull Requests

**Jobs:**
- **Lint** - Code quality checks using golangci-lint and gofmt
- **Test** - Run all tests with race detector and coverage reporting
- **Security** - Security scanning with govulncheck and Trivy

**Environment:** PostgreSQL 16, Redis 7

### 2. Docker Build & Push
**File:** `.github/workflows/docker.yml`  
**Triggers:** Push to `main`, Tags (`v*.*.*`), Pull Requests

**Jobs:**
- Build multi-architecture Docker images (linux/amd64, linux/arm64)
- Push to GitHub Container Registry (ghcr.io)
- Security scan with Trivy
- Automatic tagging:
  - `latest` - latest commit on main branch
  - `v1.2.3` - semantic version tags
  - `main-sha123456` - branch name with commit SHA

### 3. Dependencies Update
**File:** `.github/workflows/dependencies.yml`  
**Triggers:** Weekly schedule (Monday 9:00 AM UTC), Manual dispatch

**Jobs:**
- Update all Go dependencies
- Run vulnerability checks
- Create automated Pull Request with updates

### 4. Pull Request Checks
**File:** `.github/workflows/pr-checks.yml`  
**Triggers:** Pull Request events

**Jobs:**
- **PR Validation** - Check PR title and commit messages (Conventional Commits)
- **Changes Detection** - Detect what files were changed
- **Test Go Changes** - Run tests only for changed packages
- **Test Docker Changes** - Build Docker image if Dockerfile changed
- **PR Size Check** - Warn about large PRs (>1000 lines)

### 5. Release
**File:** `.github/workflows/release.yml`  
**Triggers:** Tags (`v*.*.*`)

**Jobs:**
- Generate changelog from commits
- Build binaries for multiple platforms:
  - Linux (amd64, arm64)
  - macOS (amd64, arm64)
  - Windows (amd64)
- Create GitHub Release with binaries and changelog
- Deploy to production (if not pre-release)

## Required Secrets

Configure these secrets in your GitHub repository settings:

### Optional (for enhanced features)
- `CODECOV_TOKEN` - For code coverage reporting to Codecov
- `PRODUCTION_HOST` - Production server hostname
- `PRODUCTION_USER` - Production server SSH user
- `PRODUCTION_SSH_KEY` - Production server SSH key

## Branch Protection Rules

Recommended branch protection rules for `main`:

1. ✅ Require status checks to pass before merging:
   - Lint
   - Test
   - Security Scan
   - PR Validation

2. ✅ Require branches to be up to date before merging

3. ✅ Require linear history

4. ✅ Include administrators

## Badges

Add these badges to your README.md:

```markdown
[![CI](https://github.com/vladpann/go-cloud-file-storage/actions/workflows/ci.yml/badge.svg)](https://github.com/vladpann/go-cloud-file-storage/actions/workflows/ci.yml)
[![Docker](https://github.com/vladpann/go-cloud-file-storage/actions/workflows/docker.yml/badge.svg)](https://github.com/vladpann/go-cloud-file-storage/actions/workflows/docker.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/vladpann/go-cloud-file-storage)](https://goreportcard.com/report/github.com/vladpann/go-cloud-file-storage)
[![codecov](https://codecov.io/gh/vladpann/go-cloud-file-storage/branch/main/graph/badge.svg)](https://codecov.io/gh/vladpann/go-cloud-file-storage)
```

## Usage Examples

### Manual Workflow Dispatch

Some workflows can be triggered manually:

```bash
# Trigger dependencies update
gh workflow run dependencies.yml

# Trigger release (requires tag)
git tag v1.0.0
git push origin v1.0.0
```

### Creating a Release

1. Update version in code if needed
2. Commit all changes
3. Create and push a tag:
   ```bash
   git tag -a v1.0.0 -m "Release version 1.0.0"
   git push origin v1.0.0
   ```
4. GitHub Actions will automatically:
   - Build binaries
   - Create GitHub Release
   - Build and push Docker images
   - Deploy to production (if configured)

### Conventional Commits

All commit messages must follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `test`: Adding or updating tests
- `build`: Build system changes
- `ci`: CI/CD changes
- `chore`: Other changes that don't modify src or test files
- `revert`: Revert previous commit

**Examples:**
```
feat(auth): add JWT authentication
fix(database): resolve connection pool leak
docs(readme): update installation instructions
test(users): add integration tests for sign-out
```

## Troubleshooting

### Tests Failing in CI

1. Check if all required environment variables are set
2. Verify database migrations are running successfully
3. Check for race conditions (tests run with `-race` flag)

### Docker Build Failing

1. Verify Dockerfile syntax
2. Check if all dependencies are available
3. Review Docker build logs in Actions tab

### Linter Errors

```bash
# Run linter locally
make linter-golangci

# Auto-fix formatting issues
make format
```

## Local Development

Run CI checks locally before pushing:

```bash
# Run all pre-commit checks
make pre-commit

# Individual checks
make deps           # Update dependencies
make format         # Format code
make linter-golangci # Run linter
make test           # Run tests
```
