# Project State

_Last updated: 2025-12-02_

## Current Capabilities

- Documentation framework established
- 7 Architecture Decision Records (ADRs) defining core approach
- Kanban workflow with 24 backlog items across 5 phases
- Clear distribution strategy via GitHub Releases + ghcr.io

## What Exists

- `docs/` — Project documentation (readme, decisions, project_state)
- `tasks/` — Capsule template for task specifications
- `kanban.md` — Active backlog and workflow tracking
- `CLAUDE.md` — AI guidance document
- `amp.agent.md` — Reference implementation from Amp

## What Does NOT Exist Yet

- Go source code
- Validation container (Dockerfile)
- GitHub Actions workflows
- Test suite

## Technical Decisions (ADRs)

| ADR | Decision |
|-----|----------|
| ADR-001 | Fail-fast sequential validation pipeline |
| ADR-002 | Docker-based hermetic validation |
| ADR-003 | Go implementation stack |
| ADR-004 | Agent-based architecture with **mandatory** validation |
| ADR-005 | Podman primary, Docker fallback |
| ADR-006 | GitHub Registry distribution (ghcr.io) |
| ADR-007 | Development in WSL2 on Windows 11 |

## Constraints

- **Stack:** Go (single binary, Amp-proven agent pattern)
- **Container Runtime:** Podman primary, Docker fallback
- **AI Integration:** AWS Bedrock with Claude models
  - Model IDs from environment variables
  - `global.` prefix for inference profiles
- **Architecture:** Agent-based with **mandatory validation** (not optional)
- **Distribution:** GitHub Releases (binaries) + ghcr.io (container)

## Known Risks / Issues

- Single-file limitation initially (multi-file support is complex)
- MSAN only works on Linux (not macOS or Windows containers)
- Filesystem performance slower when developing on /mnt/c/ in WSL2
- Container image size could be large (Clang + tools)

## Upcoming Work

1. **T-004:** Initialize Go module and project structure
2. **T-005:** Implement basic agent loop (Amp pattern)
3. **T-006:** Add AWS Bedrock client

See `kanban.md` for full backlog (24 items across 5 phases).

## Directory Sketch (Target)

```text
bjarne/
├── main.go               # Entry point, agent initialization
├── agent.go              # Agent loop, conversation management
├── tools.go              # Tool definitions and implementations
├── bedrock.go            # AWS Bedrock client wrapper
├── container.go          # Podman/Docker abstraction
├── validator.go          # Validation orchestration
├── parsers/
│   ├── tidy.go          # clang-tidy output parser
│   └── sanitizer.go     # ASAN/UBSAN/TSAN output parser
│
├── docker/
│   └── Dockerfile       # bjarne-validator container
│
├── .github/
│   └── workflows/
│       ├── build.yml    # Cross-platform binary builds
│       └── release.yml  # GitHub Release + ghcr.io push
│
├── docs/
│   ├── readme.md
│   ├── project_state.md
│   └── decisions.md
│
├── tasks/
│   └── capsule-template.md
│
├── kanban.md
├── CLAUDE.md
├── go.mod
├── go.sum
└── .gitignore
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        bjarne CLI                           │
│                      (Go single binary)                     │
├─────────────────────────────────────────────────────────────┤
│  Agent Loop                                                 │
│  ┌─────────┐    ┌──────────────┐    ┌─────────────────────┐│
│  │  User   │───▶│    Claude    │───▶│       Tools         ││
│  │ Prompt  │    │  (Bedrock)   │    │  read_file          ││
│  └─────────┘    └──────────────┘    │  list_files         ││
│       ▲                │            │  edit_file          ││
│       │                │            │  validate_code ─────┼┼──┐
│       │                ▼            │  write_output       ││  │
│       └────────────────────────────┘└─────────────────────┘│  │
└─────────────────────────────────────────────────────────────┘  │
                                                                 │
┌─────────────────────────────────────────────────────────────┐  │
│              bjarne-validator Container                     │◀─┘
│                    (ghcr.io)                                │
├─────────────────────────────────────────────────────────────┤
│  • Clang 18+ (clang++, clang-tidy)                         │
│  • AddressSanitizer (ASAN)                                 │
│  • UndefinedBehaviorSanitizer (UBSAN)                      │
│  • ThreadSanitizer (TSAN)                                  │
│  • MemorySanitizer (MSAN) — Linux only                     │
└─────────────────────────────────────────────────────────────┘
```

## User Flow

```
$ bjarne
bjarne v0.1.0

Checking for validation container...
Container found: ghcr.io/<org>/bjarne-validator:latest

Chat with bjarne (use 'ctrl-c' to quit)

You: write a thread-safe counter in C++

Claude: I'll create a thread-safe counter using std::atomic. Let me validate it first.

tool: validate_code({...})

Claude: The code passed all validation gates:
✓ clang-tidy (no warnings)
✓ Compilation (success)
✓ ASAN (no memory errors)
✓ UBSAN (no undefined behavior)
✓ TSAN (no data races)
✓ Runtime (executed successfully)

Here's the validated code:

```cpp
#include <atomic>
#include <iostream>

class ThreadSafeCounter {
    std::atomic<int> count{0};
public:
    void increment() { ++count; }
    void decrement() { --count; }
    int get() const { return count.load(); }
};
```

Would you like me to save this to a file?

You: yes, save to counter.cpp

tool: write_output({"path": "counter.cpp", ...})

Claude: Saved to counter.cpp
```

---

> Current phase: Planning complete. Ready for implementation (T-004).
