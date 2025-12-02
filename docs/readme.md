# bjarne

## Mission

Enable AI-assisted C/C++ development where generated code must pass rigorous validation gates (static analysis, sanitizers, compilation) before being accepted — ensuring correctness, not just compilation.

## Context

AI code generation tools produce C/C++ that compiles but often contains subtle bugs: memory errors, undefined behavior, race conditions, and security vulnerabilities. Human developers using AI assistance need a safety net that catches these issues *before* commit, not in production.

**bjarne** bridges this gap: AI generates or assists with code, validation gates verify correctness, and only proven-correct code enters the codebase.

## Scope — What It **Does**

- Accept user prompts for C/C++ code (functions, features, algorithms)
- Generate code via Claude models (AWS Bedrock)
- Validate through multi-gate pipeline (clang-tidy, ASAN, UBSAN, TSAN, MSAN)
- Iterate automatically: failed gates → AI fixes → re-validate
- Return validated code to console for user review
- Allow user to write approved code to file (.c, .cpp, later .ts, .js, etc.)
- Support validation-only mode for existing code

## Non-Goals — What It **Does Not Do**

- Replace compiler error messages (we augment, not replace)
- Support languages other than C/C++ (initially)
- Provide IDE integration (future phase)
- Handle multi-file projects with complex build systems (initially)
- Guarantee 100% bug-free code (we catch *detectable* issues)

## Success Criteria

- [ ] Single-file C++ validation passes through all gates in <30 seconds
- [ ] AI feedback loop achieves >80% success rate on medium-complexity prompts
- [ ] Pre-commit hook integration works with standard git workflow
- [ ] Clear, actionable error messages for each validation failure
- [ ] Docker-based validation ensures hermetic, reproducible results

## Constraints

- **Stack:** Go — single binary distribution, excellent process management
- **Container Runtime:** Podman primary, Docker fallback
- **Compilers:** Clang 18+ with full sanitizer support (inside container)
- **AI Integration:** Claude models via AWS Bedrock (model IDs from environment variables, `global.` prefix for inference profiles)
- **Platform:** Cross-compiled binaries for Linux, Windows, macOS

## Guardrails

- All validation gates must run in isolated Docker containers (no host system access)
- Every change updates `project_state.md`
- ADRs required for validation gate additions or architectural changes
- Code that fails any gate cannot be committed (pre-commit hook enforcement)

---

> "Within C++, there is a much smaller and cleaner language struggling to get out."
> — Bjarne Stroustrup
>
> **bjarne** helps that cleaner code emerge by catching the mistakes before they become permanent.
