# Bjarne VSCode Extension

AI-assisted C/C++ code generation with mandatory validation gates.

## Features

- **Code Generation**: Generate C/C++ code from natural language prompts
- **Validation**: Validate current file through sanitizer gates (ASAN, UBSAN, TSAN, MSan)
- **Diagnostics**: See validation errors directly in the editor
- **Chat Panel**: Interactive chat interface for code assistance

## Commands

- `Bjarne: Generate Code from Prompt` - Generate new code from a description
- `Bjarne: Validate Current File` - Run validation gates on the active file
- `Bjarne: Open Chat Panel` - Open interactive chat interface

## Requirements

- [bjarne CLI](https://github.com/3rg0n/bjarne) installed and in PATH
- Docker or Podman for validation container
- AWS credentials (for Bedrock) or API keys for other providers

## Configuration

| Setting | Description | Default |
|---------|-------------|---------|
| `bjarne.binaryPath` | Path to bjarne binary | (uses PATH) |
| `bjarne.containerImage` | Validation container image | `ghcr.io/3rg0n/bjarne-validator:latest` |
| `bjarne.provider` | LLM provider | `bedrock` |

## Development

```bash
cd vscode-extension
npm install
npm run compile
```

To test:
1. Open the extension folder in VSCode
2. Press F5 to launch Extension Development Host
3. Open a C/C++ file and run commands from Command Palette

## Building VSIX

```bash
npm run package
```

This creates `bjarne-x.x.x.vsix` which can be installed manually.
