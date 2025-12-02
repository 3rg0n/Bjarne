# Kanban Board

_Last updated: 2025-12-02_

**Project:** bjarne — AI-Assisted C/C++ Development with Validation Gates

---

## Done

| ID | Task | Completed |
|----|------|-----------|
| T-000 | Repository scaffold and documentation framework | 2025-12-02 |
| T-001 | Define project vision and populate docs/readme.md | 2025-12-02 |
| T-002 | Populate docs/project_state.md with initial state | 2025-12-02 |
| T-003 | Record ADRs 001-007: Core architecture decisions | 2025-12-02 |

---

## Doing

| ID | Task | Started | Owner |
|----|------|---------|-------|
| | (No tasks in progress) | | |

---

## Backlog

### Phase 1: Foundation (High Priority)

| ID | Task | Dependencies |
|----|------|--------------|
| T-004 | Initialize Go module and project structure | T-003 |
| T-005 | Implement basic agent loop (Amp pattern) | T-004 |
| T-006 | Add AWS Bedrock client with env var config | T-004 |
| T-007 | Implement `read_file` tool | T-005 |
| T-008 | Implement `list_files` tool | T-005 |
| T-009 | Implement `edit_file` tool | T-005 |

### Phase 2: Validation Container (High Priority)

| ID | Task | Dependencies |
|----|------|--------------|
| T-010 | Create Dockerfile with Clang 18+ and sanitizers | T-004 |
| T-011 | Implement container runtime detection (podman/docker) | T-004 |
| T-012 | Implement `validate_code` tool (orchestrates container) | T-010, T-011 |
| T-013 | Parse clang-tidy output for AI consumption | T-012 |
| T-014 | Parse sanitizer output (ASAN/UBSAN/TSAN) for AI | T-012 |

### Phase 3: User Experience (Medium Priority)

| ID | Task | Dependencies |
|----|------|--------------|
| T-015 | Implement `write_output` tool (save to file) | T-009 |
| T-016 | Add system prompt for C/C++ code generation | T-005 |
| T-017 | First-run container pull experience | T-011 |
| T-018 | Add --version and --help flags | T-004 |
| T-019 | Error handling and user-friendly messages | T-005 |

### Phase 4: Distribution (Medium Priority)

| ID | Task | Dependencies |
|----|------|--------------|
| T-020 | GitHub Actions: Build cross-platform binaries | T-018 |
| T-021 | GitHub Actions: Build and push container to ghcr.io | T-010 |
| T-022 | Create GitHub Release workflow (on tag) | T-020, T-021 |
| T-023 | Write installation documentation | T-022 |

### Phase 5: Polish (Low Priority)

| ID | Task | Dependencies |
|----|------|--------------|
| T-024 | Add iteration limits / token budget | T-012 |
| T-025 | Model selection via env vars or flags | T-006 |
| T-026 | Validate-only mode (existing code) | T-012 |
| T-027 | Colored terminal output | T-005 |

---

## Icebox (Future Considerations)

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

**Phase 1: Foundation** (Next)
- [x] Project vision defined (T-001)
- [x] Architecture decisions recorded (T-003)
- [ ] Go module initialized (T-004)
- [ ] Basic agent loop (T-005)
- [ ] Core tools (T-007, T-008, T-009)

**Phase 2: Validation** (Blocked by Phase 1)
- [ ] Dockerfile (T-010)
- [ ] Container runtime (T-011)
- [ ] validate_code tool (T-012)

**Phase 3: UX** (Blocked by Phase 2)
- [ ] write_output tool (T-015)
- [ ] System prompt (T-016)

**Phase 4: Distribution** (Blocked by Phase 3)
- [ ] Cross-platform builds (T-020)
- [ ] ghcr.io container (T-021)

---

## Architecture Summary

```
┌─────────────────────────────────────────────────────────────┐
│                        bjarne CLI                           │
│                      (Go single binary)                     │
├─────────────────────────────────────────────────────────────┤
│  Agent Loop (Amp pattern)                                   │
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

---

> "Within C++, there is a much smaller and cleaner language struggling to get out."
> — Bjarne Stroustrup
>
> **bjarne** helps that cleaner code emerge by catching the mistakes before they become permanent.
