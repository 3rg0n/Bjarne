# Kanban Board

_Last updated: 2025-12-03_

**Project:** bjarne — AI-Assisted C/C++ Development with Validation Gates

---

## Done

| ID | Task | Completed |
|----|------|-----------|
| T-000 | Repository scaffold and documentation framework | 2025-12-02 |
| T-001 | Define project vision and populate docs/readme.md | 2025-12-02 |
| T-002 | Populate docs/project_state.md with initial state | 2025-12-02 |
| T-003 | Record ADRs 001-007: Core architecture decisions | 2025-12-02 |
| T-004 | Initialize Go module and project structure | 2025-12-02 |
| T-005 | Implement interactive REPL loop (TTY interface) | 2025-12-02 |
| T-006 | Add AWS Bedrock client with env var config | 2025-12-02 |
| T-007 | Implement code generation prompt → Bedrock | 2025-12-02 |
| T-008 | Create Dockerfile with Clang 18+ and sanitizers | 2025-12-02 |
| T-009 | Implement container runtime detection (podman/docker) | 2025-12-02 |
| T-010 | Implement validation pipeline (clang-tidy → ASAN → UBSAN → TSAN) | 2025-12-02 |
| T-011 | Parse clang-tidy output for display | 2025-12-03 |
| T-012 | Parse sanitizer output (ASAN/UBSAN/TSAN) for display | 2025-12-03 |
| T-014 | Implement iteration loop (validation fails → re-generate) | 2025-12-03 |
| T-017 | First-run container pull experience | 2025-12-03 |
| T-019 | Error handling and user-friendly messages | 2025-12-03 |
| T-022 | GitHub Actions for cross-platform binaries | 2025-12-03 |
| T-023 | GitHub Actions for container image | 2025-12-03 |

---

## Doing

| ID | Task | Started | Owner |
|----|------|---------|-------|
| | (No tasks in progress) | | |

---

## Backlog

### Phase 1: Foundation — COMPLETE

All Phase 1 tasks completed. Ready for Phase 2 (Validation Container).

### Phase 2: Validation Container — COMPLETE

All Phase 2 tasks completed.

### Phase 3: Core Workflow — COMPLETE

All core workflow tasks completed (T-013, T-015, T-016 were implemented as part of earlier tasks).

### Phase 4: User Experience — COMPLETE

| ID | Task | Dependencies | Status |
|----|------|--------------|--------|
| T-017 | First-run container pull experience | T-009 | ✓ Done |
| T-018 | Add --version and --help flags | T-004 | ✓ Done (T-004) |
| T-019 | Error handling and user-friendly messages | T-005 | ✓ Done |
| T-020 | Colored terminal output | T-005 | ✓ Done (throughout) |
| T-021 | Add `/` command menu | T-005 | ✓ Done (T-005) |

### Phase 5: Distribution — MOSTLY COMPLETE

| ID | Task | Dependencies | Status |
|----|------|--------------|--------|
| T-022 | GitHub Actions: Build cross-platform binaries | T-018 | ✓ Done |
| T-023 | GitHub Actions: Build and push container to ghcr.io | T-008 | ✓ Done |
| T-024 | Create GitHub Release workflow (on tag) | T-022, T-023 | ✓ Done (in release.yml) |
| T-025 | Write installation documentation | T-024 | Pending |

### Phase 6: Polish (Low Priority)

| ID | Task | Dependencies |
|----|------|--------------|
| T-026 | Add iteration limits / token budget | T-014 |
| T-027 | Model selection via env vars or flags | T-006 |
| T-028 | Validate-only mode (existing code) | T-010 |

---

## Icebox (Future Considerations)

### Security & Quality
| ID | Task | Notes |
|----|------|-------|
| T-029 | Integrate llm-guard for prompt scanning | Prompt injection, secrets, toxicity detection |
| T-030 | Integrate codeguard safe-c-functions rules into system prompt | Ban unsafe functions |
| T-031 | Add toolchain hardening flags to validation container | -fstack-protector-all, PIE, RELRO |

### Core Enhancements
| ID | Task | Notes |
|----|------|-------|
| F-001 | Multi-file project validation | Requires dependency analysis |
| F-002 | External library support | Header-only vs linked |
| F-003 | IDE integration (VSCode, CLion) | After CLI stable |
| F-004 | Web interface | After CLI stable |
| F-005 | CI/CD pipeline integration | GitHub Actions, GitLab CI |
| F-006 | AWS Lambda/Fargate deployment | Commercialization phase |
| F-007 | Support for other languages (.ts, .js, .py) | After C/C++ stable |

### Domain-Specific Validation Gates (Future)
| ID | Domain | Validation Types |
|----|--------|------------------|
| F-010 | Game Development | Frame timing, memory budget, shader compilation |
| F-011 | High-Frequency Trading | Latency p55/p75/p99, lock-free verification, cache optimization |
| F-012 | Embedded Systems | Stack size limits, interrupt safety, real-time constraints |
| F-013 | Security | Fuzzing, static security analysis, input validation |
| F-014 | Performance | Benchmark comparison, memory profiling, CPU profiling |

---

## WIP Limits

- **Doing:** Maximum 3 items
- **Rule:** Complete or move to backlog before starting new work

## Workflow

1. **Pull from Backlog** → Move task to Doing, record start date
2. **Work on Task** → Follow capsule specification if exists
3. **Complete Task** → Update docs, commit with T-XXX reference, move to Done
4. **Commit Rule:** Every completed task requires immediate git commit

---

## Progress Summary

**Phase 1: Foundation** — COMPLETE
- [x] Project vision defined (T-001)
- [x] Architecture decisions recorded (T-003)
- [x] Go module initialized (T-004)
- [x] Interactive REPL with /commands (T-005)
- [x] Bedrock client + code generation (T-006, T-007)

**Phase 2: Validation** — COMPLETE
- [x] Dockerfile with Wolfi + Ubuntu fallback (T-008)
- [x] Container runtime detection (T-009)
- [x] Validation pipeline integration (T-010)
- [x] Diagnostic parsers for clang-tidy and sanitizers (T-011, T-012)

**Phase 3: Core Workflow** — COMPLETE
- [x] Full flow integration (implemented with T-005, T-010)
- [x] Iteration loop (T-014)
- [x] Save command (implemented with T-005)
- [x] System prompt for C/C++ (implemented with T-007)

**Phase 4: User Experience** — COMPLETE
- [x] First-run container pull (T-017)
- [x] --version and --help flags (implemented with T-004)
- [x] Error handling with suggestions (T-019)
- [x] Colored terminal output (implemented throughout)
- [x] /command menu (implemented with T-005)

**Phase 5: Distribution** — MOSTLY COMPLETE
- [x] GitHub Actions for CI (ci.yml)
- [x] GitHub Actions for cross-platform binaries (release.yml)
- [x] GitHub Actions for container image (container.yml)
- [ ] Installation documentation (T-025)

---

## Architecture Summary

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
│                         (ghcr.io)                               │
├─────────────────────────────────────────────────────────────────┤
│  Pipeline: clang-tidy → compile → ASAN → UBSAN → [TSAN] → run  │
│                                                                 │
│  • Clang 18+ (clang++, clang-tidy)                             │
│  • AddressSanitizer (ASAN)                                     │
│  • UndefinedBehaviorSanitizer (UBSAN)                          │
│  • ThreadSanitizer (TSAN) — if threads detected                │
│  • MemorySanitizer (MSAN) — Linux only                         │
└─────────────────────────────────────────────────────────────────┘
```

**Flow:**
```
User Prompt
    │
    ▼
bjarne calls Bedrock (Claude generates code)
    │
    ▼
bjarne runs validation pipeline in container
    │
    ├── FAIL → bjarne sends errors back to Claude → iterate
    │
    └── PASS → bjarne displays validated code to user
                    │
                    ▼
              User: /save filename.cpp
```

---

> "Within C++, there is a much smaller and cleaner language struggling to get out."
> — Bjarne Stroustrup
>
> **bjarne** helps that cleaner code emerge by catching the mistakes before they become permanent.
