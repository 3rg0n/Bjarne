# bjarne

AI-assisted C/C++ code generation with mandatory validation.

> "Within C++, there is a much smaller and cleaner language struggling to get out."
> — Bjarne Stroustrup

**bjarne** helps that cleaner code emerge by catching mistakes before they become permanent.

## What It Does

1. You describe what you want in plain English
2. AI generates C/C++ code
3. Code passes through validation gates (static analysis, sanitizers, compilation)
4. Failed gates → AI fixes automatically → re-validate
5. Only validated code is presented to you

**Validation Pipeline:**
- clang-tidy (static analysis)
- cppcheck (deep static analysis)
- IWYU (Include What You Use - header hygiene)
- Compilation with hardening flags (-fstack-protector-all, FORTIFY_SOURCE, PIE)
- ASAN (AddressSanitizer - memory errors, buffer overflows, use-after-free)
- UBSAN (UndefinedBehaviorSanitizer - signed overflow, null pointer dereference)
- MSAN (MemorySanitizer - uninitialized memory reads)
- TSAN (ThreadSanitizer - data races, deadlocks) *when threads detected*

## Installation

### Linux / macOS

```bash
curl -fsSL https://raw.githubusercontent.com/3rg0n/bjarne/master/install.sh | bash
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/3rg0n/bjarne/master/install.ps1 | iex
```

### Manual Download

Download binaries from [GitHub Releases](https://github.com/3rg0n/bjarne/releases):
- `bjarne-linux-amd64` / `bjarne-linux-arm64`
- `bjarne-darwin-amd64` / `bjarne-darwin-arm64`
- `bjarne-windows-amd64.exe`

### Requirements

- **Container Runtime**: [Podman](https://podman.io/) (recommended) or Docker
  - Linux: `sudo apt install podman` or `sudo dnf install podman`
  - macOS: `brew install podman && podman machine init && podman machine start`
  - Windows: `winget install RedHat.Podman`
- **LLM API Access** (one of):
  - AWS Bedrock (default) - requires `aws configure` or environment credentials
  - Anthropic API - set `BJARNE_PROVIDER=anthropic` and `BJARNE_API_KEY`
  - OpenAI API - set `BJARNE_PROVIDER=openai` and `BJARNE_API_KEY`
  - Google Gemini - set `BJARNE_PROVIDER=gemini` and `BJARNE_API_KEY`

## Quick Start

```bash
# Start interactive mode
bjarne

# First run will pull the validation container (~500MB)
```

Then just describe what you want:

```
> write a function that checks if a number is prime

Generating code...
Validating...
  clang-tidy  PASS (0.8s)
  cppcheck    PASS (0.3s)
  compile     PASS (0.5s)
  asan        PASS (0.2s)
  ubsan       PASS (0.2s)
  msan        PASS (0.3s)
  run         PASS (0.1s)

All validation gates passed!
```

## Commands

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/model [haiku\|sonnet\|opus]` | Switch AI model (cost/capability tradeoff) |
| `/save <filename>` | Save last generated code to file |
| `/validate` | Validate code from clipboard or paste |
| `/init` | Index current workspace for context-aware generation |
| `/clear` | Clear conversation history |
| `/quit` or `Ctrl+C` | Exit |

## Configuration

Environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `BJARNE_PROVIDER` | LLM provider: `bedrock`, `anthropic`, `openai`, `gemini` | `bedrock` |
| `BJARNE_API_KEY` | API key (required for non-Bedrock providers) | - |
| `BJARNE_MODEL` | Default model: `haiku`, `sonnet`, `opus` | `sonnet` |
| `BJARNE_VALIDATOR_IMAGE` | Custom validator container image | `ghcr.io/3rg0n/bjarne-validator:latest` |
| `BJARNE_ASCII` | Use ASCII box characters (`0` or `1`) | `1` on macOS |
| `AWS_REGION` | AWS region for Bedrock | `us-west-2` |

### Model Selection

| Model | Speed | Cost | Best For |
|-------|-------|------|----------|
| `haiku` | Fast | Low | Simple functions, quick iterations |
| `sonnet` | Medium | Medium | Most tasks (recommended default) |
| `opus` | Slow | High | Complex algorithms, architecture |

## How Validation Works

bjarne runs your code through multiple validation stages in an isolated container:

1. **Static Analysis** - clang-tidy and cppcheck catch common bugs and style issues
2. **Compilation** - Strict warnings (`-Wall -Wextra -Werror`) plus security hardening
3. **Runtime Sanitizers** - Each catches different bug classes:
   - ASAN: Buffer overflows, use-after-free, double-free
   - UBSAN: Integer overflow, null dereference, alignment issues
   - MSAN: Uninitialized memory reads
   - TSAN: Data races (only when threading detected)

If any stage fails, bjarne sends the error back to the AI with guidance on how to fix it. This loop continues (up to 15 attempts with model escalation) until the code passes all gates.

## License

[Business Source License 1.1](LICENSE)

- **Free for**: Personal projects, learning, research, academic use, non-profits, internal evaluation
- **Commercial production use**: Requires a license - contact 62959009+3rg0n@users.noreply.github.com
- **Change Date**: December 8, 2028 → Apache 2.0

## Building from Source

```bash
# Clone
git clone https://github.com/3rg0n/bjarne.git
cd bjarne

# Build (standard)
go build -o bjarne .

# Build with ONNX support (enables /init workspace indexing with embeddings)
go build -tags onnx -o bjarne .
```

Requires Go 1.22+.

## Contributing

Contributions welcome! Please read the license terms - contributions are accepted under the same BSL 1.1 license.

---

*Named after Bjarne Stroustrup, creator of C++.*
