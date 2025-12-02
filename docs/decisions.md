# Decisions Log

Each entry = one irreversible or significant decision.

---

## [2025-12-02] ADR-001: Validation Gate Architecture — Fail-Fast Sequential Pipeline

**Context:**
bjarne needs to validate C/C++ code through multiple tools (clang-tidy, sanitizers, runtime execution). The key architectural question: should gates run in parallel, sequential, or some hybrid approach?

**Decision:**
Adopt a **fail-fast sequential pipeline** where gates run in order of detection speed and likelihood of catching issues. Execution stops at the first failure.

**Gate Order:**
1. clang-tidy (static analysis — catches 60-90% of issues, fast)
2. Compilation (syntax errors, type errors)
3. ASAN (AddressSanitizer — memory safety)
4. UBSAN (UndefinedBehaviorSanitizer)
5. TSAN (ThreadSanitizer — only if code uses threads)
6. Runtime execution (actual behavior verification)

**Alternatives Considered:**
- **Parallel execution** — Rejected: Wastes resources running expensive sanitizers on code that fails basic static analysis. Also complicates error feedback for AI.
- **User-configurable order** — Rejected for MVP: Adds complexity without clear benefit. Can revisit if needed.
- **All gates always run** — Rejected: TSAN is expensive and only relevant for threaded code. Conditional execution is more efficient.

**Consequences:**
- **Positive:** Fast feedback on common issues (static analysis catches most problems early)
- **Positive:** Resource-efficient (expensive gates only run on code that passes cheap gates)
- **Positive:** Clear, focused error messages (one failure at a time for AI to fix)
- **Negative:** Sequential execution means total time is sum of all gates (acceptable for correctness focus)
- **Follow-up:** Need thread detection logic to conditionally enable TSAN

---

## [2025-12-02] ADR-002: Docker-Based Hermetic Validation

**Context:**
Validation requires specific compiler versions, sanitizer support, and tool availability. Host system variation (Windows, macOS, different Linux distros) creates inconsistency.

**Decision:**
All validation gates run inside Docker containers. The host system only needs Docker installed; all tooling lives in the container.

**Container Spec:**
- Base: Ubuntu 24.04 LTS
- Clang 18+ with full sanitizer support
- clang-tidy with all checks
- No network access during validation (`--network none`)
- Read-only source mounts
- Timeout protection (default: 2 minutes)

**Alternatives Considered:**
- **Host-native tools** — Rejected: Inconsistent environments, version skew, missing sanitizer support on some platforms
- **VM-based isolation** — Rejected: Too heavy, slow startup, complex management
- **Podman instead of Docker** — Acceptable alternative, but Docker has broader adoption. Can support both.

**Consequences:**
- **Positive:** Reproducible validation across all developer machines
- **Positive:** Security isolation (malicious code can't escape container)
- **Positive:** Easy version pinning and updates
- **Negative:** Docker dependency (must be installed and running)
- **Negative:** Container startup overhead (~1-2 seconds)
- **Follow-up:** Create Dockerfile with all required tools

---

## [2025-12-02] ADR-003: Go Implementation Stack

**Context:**
Need to choose implementation language for bjarne CLI and orchestration logic. Options include C++, Rust, Go, Python, Node.js/TypeScript.

**Decision:**
Use **Go** for the entire implementation.

**Rationale:**
- **Single binary distribution** — No runtime dependencies. Users download one executable.
- **Excellent process management** — Spawning containers, handling stdout/stderr, timeouts are clean in Go
- **Cross-compilation trivial** — `GOOS=windows GOARCH=amd64 go build`
- **Proven pattern** — Amp (ampcode.com) demonstrates a full code-editing agent in < 400 lines of Go
- **AWS SDK v2** — Excellent Bedrock support for Claude models
- **Future AWS deployment** — Go services are easy to containerize and scale

**Alternatives Considered:**
- **Node.js/TypeScript** — Rejected: Runtime dependency, cpp-forge context was different (Ink UI needed)
- **C++** — Rejected: Ironic for a C++ validation tool to have complex build requirements
- **Rust** — Rejected: Excellent language but steeper learning curve, smaller CLI ecosystem
- **Python** — Rejected: Type safety concerns, dependency management complexity

**Consequences:**
- **Positive:** Single binary, easy distribution
- **Positive:** Fast startup, low memory footprint
- **Positive:** Goroutines simplify concurrent container orchestration
- **Negative:** Slightly longer iteration cycle (compile step)
- **Follow-up:** Set up Go module with AWS SDK v2, Anthropic SDK (or Bedrock equivalent)

---

## [2025-12-02] ADR-004: Agent-Based Architecture with Mandatory Validation

**Context:**
Originally planned a sequential pipeline where we orchestrate: generate → validate → feedback → iterate. After reviewing Amp's architecture, a simpler agent pattern emerged. However, unlike Amp where tool use is optional, **validation in bjarne is mandatory** — code must pass all gates before being presented to the user.

**Decision:**
Implement bjarne as an **agent with mandatory validation**. Claude handles code generation and iteration, but the system **enforces** validation before any code can be returned to the user.

**Key Constraint:** Validation is NOT optional. The system must:
1. Intercept any code Claude generates
2. Run it through all validation gates
3. Only present validated code to the user
4. Force iteration on validation failures

**Core Tools:**
1. `read_file` — Read existing code files
2. `list_files` — Explore codebase structure
3. `edit_file` — Create/modify code (string replacement)
4. `validate_code` — **MANDATORY** — Run validation gates, return structured errors
5. `write_output` — Write approved code to user-specified file (only after validation passes)

**Enforcement Mechanism:**
- System prompt instructs Claude that ALL generated code MUST be validated
- `write_output` tool checks validation status — refuses to write unvalidated code
- Consider: intercept code blocks in Claude's response and auto-validate

**Flow:**
```
User: "write a thread-safe counter in C++"
       ↓
Claude: generates code, MUST call validate_code (enforced by system prompt)
       ↓
Tool: runs clang-tidy, ASAN, UBSAN, TSAN → returns errors
       ↓
Claude: sees errors, fixes code, calls validate_code again (mandatory)
       ↓
Tool: all gates pass → returns success with validation token
       ↓
Claude: presents VALIDATED code to user, asks if they want to save
       ↓
User: "yes, save to counter.cpp"
       ↓
Claude: calls write_output (checks validation token)
```

**Alternatives Considered:**
- **Optional validation (Amp style)** — Rejected: Defeats the core purpose of bjarne
- **Full pipeline orchestration** — Rejected: Unnecessary complexity if we enforce via system prompt
- **Post-response validation** — Considered: Intercept Claude's response and validate any code blocks automatically

**Consequences:**
- **Positive:** Guarantees all returned code passes validation
- **Positive:** Still leverages Claude's iteration intelligence
- **Positive:** Clear separation: Claude generates, system validates
- **Negative:** System prompt must be carefully crafted to enforce validation
- **Follow-up:** Design system prompt that makes validation non-negotiable

**Future Extensibility:**
The validation system is designed to be extensible. Future validation types:
- **Game Development:** Frame timing, memory budget, shader compilation
- **High-Frequency Trading:** Latency checks (p55/p75/p99), lock-free verification
- **Embedded Systems:** Stack size, interrupt safety, real-time constraints
- **Security:** Fuzzing, static security analysis, input validation

Each domain can add its own validation gates while keeping the core mandatory validation architecture.

---

## [2025-12-02] ADR-005: Podman Primary, Docker Fallback

**Context:**
Need container runtime for hermetic validation. Both Podman and Docker are viable options.

**Decision:**
Use **Podman as primary runtime**, fall back to Docker if Podman not available.

**Rationale:**
- **Daemonless** — No background process required
- **Rootless by default** — Better security posture
- **CLI compatible** — Same commands work: `podman run` vs `docker run`
- **OCI compliant** — Same images work with either runtime
- **Future AWS** — Both use OCI images, same Dockerfile works in ECS/Fargate

**Detection Logic:**
```go
func getContainerRuntime() string {
    if _, err := exec.LookPath("podman"); err == nil {
        return "podman"
    }
    if _, err := exec.LookPath("docker"); err == nil {
        return "docker"
    }
    return "" // error: no container runtime found
}
```

**Consequences:**
- **Positive:** Works on systems with only Podman (RHEL, Fedora default)
- **Positive:** Works on systems with only Docker (most dev machines)
- **Positive:** Better security with Podman's rootless default
- **Negative:** Need to test with both runtimes
- **Follow-up:** Abstract container commands behind interface

---

## [2025-12-02] ADR-006: GitHub Registry Distribution

**Context:**
Users need easy installation of both the CLI binary and the validation container. Manual building is a barrier to adoption.

**Decision:**
Distribute via **GitHub Releases and GitHub Container Registry (ghcr.io)**:

1. **CLI Binary** — GitHub Releases with cross-platform builds
2. **Validation Container** — GitHub Container Registry (ghcr.io)

**Distribution Strategy:**

```
# Install CLI (user chooses platform)
# Windows
curl -L https://github.com/<org>/bjarne/releases/latest/download/bjarne-windows-amd64.exe -o bjarne.exe

# macOS (Intel)
curl -L https://github.com/<org>/bjarne/releases/latest/download/bjarne-darwin-amd64 -o bjarne

# macOS (Apple Silicon)
curl -L https://github.com/<org>/bjarne/releases/latest/download/bjarne-darwin-arm64 -o bjarne

# Linux
curl -L https://github.com/<org>/bjarne/releases/latest/download/bjarne-linux-amd64 -o bjarne

# Pull validation container (automatic on first run, or manual)
podman pull ghcr.io/<org>/bjarne-validator:latest
# or
docker pull ghcr.io/<org>/bjarne-validator:latest
```

**Build Matrix:**
| OS | Architecture | Binary Name |
|----|--------------|-------------|
| Windows | amd64 | bjarne-windows-amd64.exe |
| macOS | amd64 | bjarne-darwin-amd64 |
| macOS | arm64 | bjarne-darwin-arm64 |
| Linux | amd64 | bjarne-linux-amd64 |
| Linux | arm64 | bjarne-linux-arm64 |

**GitHub Actions Workflow:**
- On tag push (v*), build all binaries using Go cross-compilation
- Build and push container to ghcr.io
- Create GitHub Release with all binaries attached

**First Run Experience:**
```
$ bjarne
bjarne v0.1.0

Checking for validation container...
Container not found. Pulling ghcr.io/<org>/bjarne-validator:latest...
[====================================] Done!

Chat with bjarne (use 'ctrl-c' to quit)
You:
```

**Alternatives Considered:**
- **Homebrew/apt/chocolatey** — Rejected for MVP: Adds package maintainer burden
- **Docker Hub** — Rejected: ghcr.io integrates better with GitHub, free for public repos
- **Self-hosted registry** — Rejected: Unnecessary complexity

**Consequences:**
- **Positive:** Single command install on any platform
- **Positive:** Container auto-pulled on first run
- **Positive:** Version pinning possible (specific tags)
- **Positive:** Free hosting for open source
- **Negative:** Users need curl/wget (trivial on all platforms)
- **Follow-up:** Set up GitHub Actions workflow for releases

---

## [2025-12-02] ADR-007: Development Environment — Windows 11 + WSL2

**Context:**
Primary development machine is Windows 11 with WSL2. Need to ensure smooth development experience across native Windows and Linux environments.

**Decision:**
Develop primarily in **WSL2 (Ubuntu)**, with Windows-native testing for cross-platform verification.

**Rationale:**
- WSL2 provides native Linux environment for Podman/Docker
- Go cross-compilation works seamlessly from Linux
- Clang/sanitizers behave identically to target Linux containers
- Can test Windows binary directly from WSL2 via `/mnt/c/`

**Development Workflow:**
```bash
# In WSL2
cd /mnt/c/dev/Github/bjarne  # Or clone to WSL filesystem for speed

# Build all platforms
GOOS=linux GOARCH=amd64 go build -o dist/bjarne-linux-amd64
GOOS=darwin GOARCH=amd64 go build -o dist/bjarne-darwin-amd64
GOOS=darwin GOARCH=arm64 go build -o dist/bjarne-darwin-arm64
GOOS=windows GOARCH=amd64 go build -o dist/bjarne-windows-amd64.exe

# Test Linux binary directly
./dist/bjarne-linux-amd64

# Test Windows binary
/mnt/c/dev/Github/bjarne/dist/bjarne-windows-amd64.exe
```

**Container Testing:**
```bash
# Build container in WSL2
podman build -t bjarne-validator:dev ./docker/

# Test validation
podman run --rm -v $(pwd)/test:/src:ro bjarne-validator:dev clang-tidy /src/test.cpp
```

**Consequences:**
- **Positive:** Linux-native development experience
- **Positive:** Easy cross-platform testing
- **Positive:** Matches CI/CD environment (GitHub Actions uses Linux runners)
- **Negative:** Filesystem performance slower on /mnt/c/ (mitigate by cloning to WSL filesystem)
- **Follow-up:** Document WSL2 setup requirements

---
