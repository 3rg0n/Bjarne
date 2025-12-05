# Installation Guide

## Quick Start

### 1. Download bjarne

Download the latest release for your platform from [GitHub Releases](https://github.com/3rg0n/bjarne/releases):

| Platform | Download |
|----------|----------|
| Linux (x64) | `bjarne-linux-amd64` |
| Linux (ARM64) | `bjarne-linux-arm64` |
| macOS (Intel) | `bjarne-darwin-amd64` |
| macOS (Apple Silicon) | `bjarne-darwin-arm64` |
| Windows (x64) | `bjarne-windows-amd64.exe` |

### 2. Make it executable (Linux/macOS)

```bash
chmod +x bjarne-*
sudo mv bjarne-* /usr/local/bin/bjarne
```

### 3. Set up AWS credentials

bjarne uses AWS Bedrock for code generation. You need valid AWS credentials:

```bash
# Option 1: Use AWS CLI
aws configure

# Option 2: Set environment variables
export AWS_ACCESS_KEY_ID=your_access_key
export AWS_SECRET_ACCESS_KEY=your_secret_key
export AWS_REGION=us-east-1
```

### 4. Run bjarne

```bash
bjarne
```

On first run, bjarne will prompt you to download the validation container (~500MB).

## Requirements

### AWS Bedrock Access

- AWS account with Bedrock access enabled
- IAM permissions for `bedrock:InvokeModel`
- Claude models enabled in your region

### Container Runtime

bjarne requires either:

- **Podman** (recommended) - [Installation guide](https://podman.io/getting-started/installation)
- **Docker** - [Installation guide](https://docs.docker.com/get-docker/)

bjarne prefers Podman (daemonless, rootless) but falls back to Docker automatically.

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `AWS_ACCESS_KEY_ID` | AWS access key | (required) |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key | (required) |
| `AWS_REGION` | AWS region | `us-east-1` |
| `BJARNE_MODEL` | Claude model ID | `global.anthropic.claude-sonnet-4-20250514-v1:0` |
| `BJARNE_VALIDATOR_IMAGE` | Custom validator image | `ghcr.io/3rg0n/bjarne-validator:latest` |

### Custom Model

To use a different Claude model:

```bash
export BJARNE_MODEL="anthropic.claude-3-5-sonnet-20241022-v2:0"
bjarne
```

Use `global.` prefix for cross-region inference profiles:

```bash
export BJARNE_MODEL="global.anthropic.claude-sonnet-4-20250514-v1:0"
```

## Building from Source

### Prerequisites

- Go 1.22 or later
- Make (optional)

### Build

```bash
git clone https://github.com/3rg0n/bjarne.git
cd bjarne
go build -o bjarne .
```

### Build with version info

```bash
go build -ldflags="-X main.Version=v1.0.0" -o bjarne .
```

### Run tests

```bash
go test ./...
```

### Run linter

```bash
golangci-lint run
```

## Building the Validation Container

If you want to build the validation container locally:

```bash
cd docker

# Build Ubuntu-based image (recommended)
podman build -f Dockerfile.ubuntu -t bjarne-validator:local .

# Or build Wolfi-based image (smaller, requires glibc compat)
podman build -t bjarne-validator:local .
```

Then set the environment variable:

```bash
export BJARNE_VALIDATOR_IMAGE=bjarne-validator:local
bjarne
```

## Troubleshooting

### "No container runtime found"

Install Podman or Docker:
- Podman: https://podman.io/getting-started/installation
- Docker: https://docs.docker.com/get-docker/

### "Failed to load AWS config"

Check your AWS credentials:
```bash
aws sts get-caller-identity
```

If this fails, run `aws configure` to set up credentials.

### "Access denied" or "Not authorized"

Your IAM user/role needs `bedrock:InvokeModel` permission. Add this policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "bedrock:InvokeModel",
      "Resource": "*"
    }
  ]
}
```

### Container pull fails

1. Check internet connectivity
2. Try pulling manually: `podman pull ghcr.io/3rg0n/bjarne-validator:latest`
3. Build locally if registry is blocked (see "Building the Validation Container")

### Validation timeout

Validation has a 2-minute timeout per stage. For complex code:
- Break into smaller pieces
- Ensure code terminates (avoid infinite loops)
- Check available system resources

## Uninstallation

### Remove binary

```bash
sudo rm /usr/local/bin/bjarne
```

### Remove container image

```bash
podman rmi ghcr.io/3rg0n/bjarne-validator:latest
# or
docker rmi ghcr.io/3rg0n/bjarne-validator:latest
```
