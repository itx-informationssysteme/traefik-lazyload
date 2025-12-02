# Docker Publishing Workflow

This project uses automated Docker image building and publishing with semantic versioning.

## How It Works

The GitHub Actions workflow (`.github/workflows/docker-publish.yml`) automatically:

1. **Builds and tests** the Go application on every push and PR
2. **Publishes Docker images** to GitHub Container Registry (ghcr.io) when:
   - Code is pushed to `master` branch
   - A pull request is merged into `master` branch

## Semantic Versioning

The project uses [semantic-release](https://github.com/semantic-release/semantic-release) with the [@esatterwhite/semantic-release-docker](https://github.com/esatterwhite/semantic-release-docker) plugin to automatically:

- Analyze commit messages (following [Conventional Commits](https://www.conventionalcommits.org/))
- Determine the next version number
- Generate release notes
- Create GitHub releases
- Build and push Docker images with proper tags

## Commit Message Format

Use conventional commit format to trigger releases:

- **fix:** patches (1.0.1)
- **feat:** minor releases (1.1.0)
- **BREAKING CHANGE:** major releases (2.0.0)
- **docs:** documentation updates (patch)
- **refactor:** code refactoring (patch)

Examples:
```
feat: add new container discovery method
fix: resolve memory leak in stats collection
feat!: redesign API interface (breaking change)
docs: update README with new configuration options
```

## Docker Image Tags

Published images are tagged as:
- `latest` - Latest release
- `v1.2.3` - Specific version
- `v1.2` - Major.minor version
- `v1` - Major version

## Using the Docker Image

```bash
# Pull the latest version
docker pull ghcr.io/itx-informationssysteme/traefik-lazyload:latest

# Pull a specific version
docker pull ghcr.io/itx-informationssysteme/traefik-lazyload:v1.2.3

# Run the container
docker run -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $(pwd)/config.yaml:/opt/app/config.yaml \
  ghcr.io/itx-informationssysteme/traefik-lazyload:latest
```

## Manual Release

To trigger a release manually, push to master with conventional commit messages or create a release through the GitHub UI.

## Configuration

The workflow configuration files:
- `.github/workflows/docker-publish.yml` - GitHub Actions workflow
- `.releaserc.json` - Semantic release configuration
- `package.json` - Node.js dependencies for semantic-release
- `.npmrc` - Local npm configuration to use public registry
- `Dockerfile` - Multi-stage Docker build optimized for Go applications

### NPM Registry Configuration

This project includes a local `.npmrc` file that ensures npm uses the public registry (`https://registry.npmjs.org/`) even if you have a private registry configured globally. This prevents conflicts with corporate or private npm registries.