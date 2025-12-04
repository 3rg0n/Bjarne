# bjarne Project State

Last updated: 2025-12-04

## Current Status

bjarne is an AI-assisted C/C++ code generation tool with mandatory validation. Phase 1 of production-grade enhancements is complete, including model escalation and bug fixes for code extraction and validation.

## Recent Commits

- `d8ab73d` - Reset escalation state at start of each generation cycle
- `c37a4fc` - Fix validation when cppcheck/lizard not installed, fix escalation display
- `08b5ba8` - Fix extractCode to handle truncated responses and Windows line endings
- `2aad5f4` - Add model escalation chain for automatic fix retries
- `2fdde57` - Add production-grade validation: cppcheck, complexity, examples, DoD

## Architecture

### Validation Pipeline (Current)

```
clang-tidy → cppcheck → IWYU → complexity → compile → ASAN → UBSAN → [TSAN] → [examples] → run
```

### Flow by Complexity

```
[EASY]    → Generate immediately (skip reflection)
[MEDIUM]  → Reflect with assumptions → Confirm → Generate
[COMPLEX] → Reflect → Clarify → Ask for Definition of Done → Parse DoD → Generate → Validate against DoD
```

### Escalation Chain (on validation failure)

```
Haiku generates → fails → Haiku fixes (2 attempts)
                       → Sonnet fixes (2 attempts)
                       → Opus fixes (2 attempts)
                       → Report failure with models tried
```

### Key Files

| File | Purpose |
|------|---------|
| `main.go` | Entry point, CLI handling |
| `tui.go` | Bubbletea TUI, state machine, escalation |
| `tui_test.go` | Tests for escalation logic |
| `container.go` | Container runtime, validation pipeline |
| `bedrock.go` | AWS Bedrock client |
| `prompts.go` | System prompts for LLM |
| `parser.go` | Diagnostic output parsing |
| `examples.go` | Example test parsing, harness generation |
| `dod.go` | Definition of Done parsing, benchmark harness |
| `config.go` | Configuration from env vars |
| `settings.go` | Settings file management, themes |
| `helpers.go` | Utility functions |
| `errors.go` | User-friendly error handling |

### TUI States

```go
StateInput          // Waiting for user input
StateThinking       // Analyzing prompt (reflection)
StateAcknowledging  // Processing user response
StateCollectingDoD  // Waiting for Definition of Done (COMPLEX only)
StateGenerating     // Generating code
StateValidating     // Running validation pipeline
StateFixing         // Attempting to fix failed code
```

## Features Implemented

### Phase 1 (Complete)

1. **Enhanced Validation**
   - cppcheck for deep static analysis
   - lizard for complexity metrics (CCN≤15, length≤100)
   - Example-based test validation
   - Benchmark harness generation

2. **Example-Based Validation**
   - Parse `func(x) -> y` patterns from prompts
   - Generate C++ test harness
   - Run as validation stage

3. **Definition of Done (COMPLEX tasks)**
   - Ask for testable acceptance criteria
   - Parse: examples, thread-safety, performance targets
   - Generate benchmark harness
   - Honest about what can/cannot be tested

4. **Enhanced Prompts**
   - Explicit assumptions in reflection
   - Complexity limits in generation prompt
   - DoDPrompt for testable requirements

5. **Model Escalation Chain**
   - Automatic fix retries on validation failure
   - 2 attempts per model before escalating
   - Haiku → Sonnet → Opus escalation
   - Tracks which models were tried
   - Shows summary on exhaustion

## Bug Fixes (Latest Session)

1. **extractCode() not working** - Fixed Windows line ending handling and truncated response support
2. **Validation failing on missing tools** - Skip cppcheck/lizard stages gracefully when not installed
3. **Escalation showing wrong model/attempt** - Fixed attempt number calculation and state reset between prompts

## Pending Work

### Immediate

1. **Rebuild container** (needs network):
   ```bash
   podman build -f docker/Dockerfile.ubuntu -t bjarne-validator:local ./docker/
   ```
   This will add cppcheck and lizard to the validation pipeline.

### Short-term

2. **Oracle mode for COMPLEX**
   - Use Opus for deeper architectural analysis
   - Generate design doc before code
   - More thorough clarification

### Long-term (from plan)

- Property-based testing (FuzzTest)
- MSan (optional, disabled by default)
- Google Benchmark integration
- Domain-specific validators
- Multi-file project support

## Configuration

Environment variables:
```
AWS_ACCESS_KEY_ID       # AWS credentials
AWS_SECRET_ACCESS_KEY   # AWS credentials
AWS_REGION              # Default: us-east-1
BJARNE_MODEL            # Claude model ID (generate model)
BJARNE_CHAT_MODEL       # Claude model ID (chat/reflection)
BJARNE_VALIDATOR_IMAGE  # Custom container image
BJARNE_MAX_ITERATIONS   # Max retry attempts (default: 3)
BJARNE_MAX_TOKENS       # Max tokens per response (default: 8192)
BJARNE_MAX_TOTAL_TOKENS # Session token budget (default: 150000)
```

Settings file: `~/.bjarne/settings.json`
```json
{
  "models": {
    "chat": "global.anthropic.claude-haiku-4-5-20251001-v1:0",
    "generate": "global.anthropic.claude-haiku-4-5-20251001-v1:0",
    "escalation": [
      "global.anthropic.claude-sonnet-4-5-20250929-v1:0",
      "global.anthropic.claude-opus-4-5-20251101-v1:0"
    ]
  },
  "validation": {
    "maxIterations": 3,
    "escalateOnFailure": true
  }
}
```

## Container

Dockerfile: `docker/Dockerfile.ubuntu`

Tools included:
- Clang 21 (clang++, clang-tidy)
- IWYU (Include What You Use)
- cppcheck
- lizard (Python)
- Sanitizers (ASAN, UBSAN, TSAN)

Build command:
```bash
podman build -f docker/Dockerfile.ubuntu -t bjarne-validator:local ./docker/
```

## Plan Document

Full production-grade plan at:
`C:\Users\ecopelan\.claude\plans\fuzzy-hopping-lemur.md`

Key sections:
- Multi-LLM Oracle Triad Architecture
- Performance Benchmarking (p55/p99)
- Cost Model
- Implementation Roadmap

## Key Insights

1. **LLMs are not true oracles** - They help create and aim tests, but execution is the source of truth

2. **Role separation matters** - Generator, Tester, Critic should be distinct

3. **Spec → Tests → Code flow** - Generate tests before code to avoid overfitting

4. **Definition of Done must be testable** - Ask for criteria bjarne can actually verify

5. **Escalation improves success** - Trying stronger models on failure catches more bugs
