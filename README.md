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
- ASAN (AddressSanitizer - memory errors)
- UBSAN (UndefinedBehaviorSanitizer)
- MSAN (MemorySanitizer - uninitialized reads)
- TSAN (ThreadSanitizer - race conditions)

## Installation

### Linux / macOS

```bash
curl -fsSL https://raw.githubusercontent.com/3rg0n/bjarne/master/install.sh | bash
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/3rg0n/bjarne/master/install.ps1 | iex
```

### Requirements

- **Container Runtime**: Podman (recommended) or Docker
- **LLM API Access**: One of:
  - AWS Bedrock (default) - requires AWS credentials
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
| `/model [haiku\|sonnet\|opus]` | Switch AI model |
| `/save <filename>` | Save last generated code |
| `/validate` | Validate code from clipboard |
| `/init` | Index current workspace for context |
| `/clear` | Clear conversation history |
| `/quit` | Exit |

## Configuration

Environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `BJARNE_PROVIDER` | LLM provider: bedrock, anthropic, openai, gemini | bedrock |
| `BJARNE_API_KEY` | API key for non-Bedrock providers | - |
| `BJARNE_MODEL` | Default model: haiku, sonnet, opus | sonnet |
| `BJARNE_VALIDATOR_IMAGE` | Custom validator container image | ghcr.io/3rg0n/bjarne-validator:latest |

## License

[Business Source License 1.1](LICENSE)

- **Free for**: Personal projects, learning, research, academic use, non-profits
- **Commercial use**: Requires a license - contact evan@copeland.dev
- **Change Date**: December 8, 2028 → Apache 2.0

## Building from Source

```bash
# Clone
git clone https://github.com/3rg0n/bjarne.git
cd bjarne

# Build
go build -o bjarne .

# Build with ONNX support (for /init workspace indexing)
go build -tags onnx -o bjarne .
```

## Contributing

Contributions welcome! Please read the license terms - contributions are accepted under the same BSL 1.1 license.
