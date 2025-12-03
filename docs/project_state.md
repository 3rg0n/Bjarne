# Project State

_Last updated: 2025-12-03_

## Current Capabilities

**bjarne is functional!** The core workflow is complete:

- **Code Generation**: Claude models via AWS Bedrock generate C/C++ code
- **Automatic Validation**: All code passes through clang-tidy → ASAN → UBSAN → TSAN pipeline
- **Iteration Loop**: Failed validation triggers automatic fix attempts (up to 3 iterations)
- **User-Friendly**: Colored output, structured diagnostics, helpful error messages
- **Cross-Platform**: Builds for Linux, macOS, Windows (amd64/arm64)

## What Exists

### Source Code
- `main.go` — Entry point with --version, --help, and REPL startup
- `repl.go` — Interactive REPL with /commands and iteration loop
- `bedrock.go` — AWS Bedrock client for Claude models
- `container.go` — Podman/Docker abstraction and validation orchestration
- `parser.go` — Diagnostic parsers for clang-tidy and sanitizers
- `prompts.go` — System prompts for code generation
- `errors.go` — User-friendly error handling with suggestions

### Tests
- `container_test.go` — Tests for thread detection, image naming, result formatting
- `repl_test.go` — Tests for code extraction and env var handling
- `parser_test.go` — Tests for diagnostic parsing
- `errors_test.go` — Tests for error formatting and suggestions

### Container
- `docker/Dockerfile` — Wolfi-based validation container (Clang 18+, sanitizers)
- `docker/Dockerfile.ubuntu` — Ubuntu fallback for glibc environments

### CI/CD
- `.github/workflows/ci.yml` — Test, lint, build on push/PR
- `.github/workflows/release.yml` — Cross-platform binary releases on tags
- `.github/workflows/container.yml` — Container image builds to ghcr.io

### Configuration
- `.golangci.yml` — Linter configuration with gosec, errcheck, etc.
- `go.mod`, `go.sum` — Go module definition and dependencies

### Documentation
- `docs/readme.md` — Project vision and scope
- `docs/decisions.md` — 7 Architecture Decision Records (ADRs)
- `docs/project_state.md` — This file
- `docs/installation.md` — Installation guide
- `kanban.md` — Task tracking
- `CLAUDE.md` — AI guidance document

## Technical Decisions (ADRs)

| ADR | Decision |
|-----|----------|
| ADR-001 | Fail-fast sequential validation pipeline |
| ADR-002 | Docker-based hermetic validation |
| ADR-003 | Go implementation stack |
| ADR-004 | CLI-orchestrated pipeline with **mandatory** validation |
| ADR-005 | Podman primary, Docker fallback |
| ADR-006 | GitHub Registry distribution (ghcr.io) |
| ADR-007 | Development in WSL2 on Windows 11 |

## Test Coverage

34 tests covering:
- Code extraction from markdown
- Thread detection in code
- Diagnostic parsing (clang-tidy, ASAN, UBSAN, TSAN)
- Result formatting
- Error handling and suggestions
- Configuration helpers

## Known Limitations

- Single-file only (no multi-file project support yet)
- MSAN only works on Linux (not in container yet)
- No streaming output during generation
- Fixed iteration limit (3 attempts)

## Remaining Work

### Phase 5 (Distribution)
- [ ] T-025: Installation documentation (in progress)

### Phase 6 (Polish)
- [ ] T-026: Add iteration limits / token budget
- [ ] T-027: Model selection via flags (env vars done)
- [ ] T-028: Validate-only mode for existing code

### Icebox
- T-029: llm-guard integration for prompt scanning
- T-030: codeguard safe-c-functions rules
- T-031: Toolchain hardening flags
- Future: Multi-file projects, IDE integration, Web interface

See `kanban.md` for full backlog.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         bjarne CLI                              │
│                    (Go single binary, TTY UI)                   │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   ┌──────────────────────────────────────────────────────────┐  │
│   │                   Interactive REPL                        │  │
│   │   You: write a thread-safe counter                       │  │
│   │   bjarne: Generating... Validating... ✓                  │  │
│   │   [code displayed]                                       │  │
│   │   You: /save counter.cpp                                 │  │
│   └──────────────────────────────────────────────────────────┘  │
│                            │                                    │
│              ┌─────────────┴─────────────┐                     │
│              ▼                           ▼                     │
│   ┌──────────────────┐        ┌──────────────────────┐        │
│   │  Bedrock Client  │        │  Validation Pipeline  │        │
│   │  (Claude models) │        │  (Container orchestr) │        │
│   └──────────────────┘        └──────────────────────┘        │
│                                          │                     │
└──────────────────────────────────────────┼─────────────────────┘
                                           │
                                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                   bjarne-validator Container                    │
│                 (ghcr.io/3rg0n/bjarne-validator)             │
├─────────────────────────────────────────────────────────────────┤
│  Pipeline: clang-tidy → compile → ASAN → UBSAN → [TSAN] → run  │
│                                                                 │
│  • Clang 18+ (clang++, clang-tidy)                             │
│  • AddressSanitizer (ASAN)                                     │
│  • UndefinedBehaviorSanitizer (UBSAN)                          │
│  • ThreadSanitizer (TSAN) — if threads detected                │
└─────────────────────────────────────────────────────────────────┘
```

---

> Current phase: **Phases 1-4 complete. Phase 5 mostly complete. Ready for Phase 6 polish.**
