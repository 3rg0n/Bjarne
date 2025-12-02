# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Quick Start

**First time here?** Read these three files in order:
1. `docs/readme.md` - Understand the project vision and scope
2. `docs/project_state.md` - See what currently exists
3. `docs/tasks.md.txt` - Check active work

**Starting a task?** Find or create a capsule in `tasks/` and follow its specifications.

## Repository Overview

This is a **template repository** implementing a structured, capsule-based development workflow designed for AI-assisted development. The repository provides a framework for building projects with clear documentation, decision tracking, and task management.

## Core Architecture

### Documentation-First Workflow

This repository uses a **three-document system** to maintain clarity and context:

1. **docs/readme.md** - Vision and scope
   - Defines mission, context, and success criteria
   - Lists what the project does and does not do
   - Should remain under one page

2. **docs/project_state.md** - Current snapshot
   - Lists current capabilities and constraints
   - Documents known risks and issues
   - Tracks upcoming work
   - **Must be updated after every significant change**

3. **docs/decisions.md** - ADR (Architecture Decision Record) log
   - Documents irreversible or significant decisions
   - Includes context, decision, alternatives, and consequences
   - **Required for all architectural decisions**

### Capsule-Based Task Management

Tasks are defined as **capsules** - self-contained work units that should take 1-2 hours of focused work.

**Capsule Structure** (see `tasks/capsule-template.md`):
- Intent: What and why
- Scope: What can/cannot be modified
- Acceptance Tests: Behavior checklist
- Technical Specification: Implementation details
- Test Requirements: Required test coverage
- Verification Checklist: Must pass before completion

**Task Tracking** (see `docs/tasks.md.txt`):
- Keep WIP ≤ 3 items active
- Move completed capsules from Active → Completed
- Commit format: `T-XXX: <summary>` (see Git Workflow section)

### Workflow Rules

When working on any task:

1. **Before starting:**
   - Read relevant capsule spec in `tasks/`
   - Check `docs/project_state.md` for current state
   - Review `docs/decisions.md` for architectural constraints

2. **During implementation:**
   - Follow scope restrictions in the capsule
   - Include tests as specified
   - Follow commit message format with capsule ID

3. **After completion:**
   - Update `docs/project_state.md` with new capabilities
   - Mark capsule as Completed in `docs/tasks.md.txt`
   - If architectural decision made, add entry to `docs/decisions.md`
   - Run all tests before marking complete

## Directory Structure

```
bjarne/
├── docs/
│   ├── readme.md          # Vision & scope (keep under 1 page)
│   ├── project_state.md   # Current snapshot (update frequently)
│   ├── decisions.md       # ADR log (for irreversible decisions)
│   └── tasks.md.txt       # Task board (WIP ≤ 3)
│
├── tasks/
│   └── capsule-template.md  # Template for new tasks
│
├── src/                   # Source code (to be created)
├── tests/                 # Tests (to be created)
└── CLAUDE.md             # This file
```

## Key Principles

1. **Every change updates PROJECT_STATE.md** - This is the source of truth for current capabilities

2. **ADRs required for irreversible decisions** - Document significant choices in `decisions.md`

3. **Capsules must be testable** - All tasks include acceptance tests and verification checklist

4. **Scope discipline** - Each capsule has explicit allowed/disallowed edits

5. **Work-in-progress limits** - Maximum 3 active capsules at once

## Anti-Patterns to Avoid

1. **Don't skip documentation updates** - `docs/project_state.md` must reflect every significant change
2. **Don't bypass capsule workflow** - Even quick fixes should have a capsule or be documented in an existing one
3. **Don't make undocumented architectural decisions** - All significant choices require ADR entries
4. **Don't exceed WIP limits** - Maximum 3 active capsules; complete or pause before starting new work
5. **Don't ignore scope boundaries** - Capsules define what can/cannot be modified for a reason
6. **Don't skip tests** - Every capsule includes test requirements that must be satisfied
7. **Don't forget commit references** - All commits must reference their capsule ID (T-XXX)
8. **Don't create orphan code** - Every feature must be documented in `project_state.md`

## Working with This Repository

### Implementing a Feature

1. **Check for existing capsule:** Look in `tasks/` for a matching task spec
2. **Create new capsule if needed:**
   ```bash
   cp tasks/capsule-template.md tasks/T-XXX-feature-name.md
   ```
   - Fill in intent, scope, acceptance tests, and technical spec
   - Add to `docs/tasks.md.txt` under Active section
   - Ensure WIP ≤ 3 items
3. **Follow the capsule:** Implement according to scope and acceptance tests
4. **Update documentation:** Modify `docs/project_state.md` with new capabilities
5. **Complete the capsule:** Mark as completed in `docs/tasks.md.txt`

### Making Architectural Decisions

When choosing between approaches (e.g., database choice, auth strategy, state management):
1. Document in `docs/decisions.md` using the ADR format
2. Include: Context, Decision, Alternatives Considered, Consequences
3. Reference the decision in commit messages and code comments

### Exploring the Codebase

1. **Vision:** Read `docs/readme.md` for project goals and scope
2. **Current state:** Check `docs/project_state.md` for capabilities and constraints
3. **History:** Review `docs/decisions.md` for architectural reasoning

## Guardrails & Quality Gates

### Pre-Implementation Checklist

Before writing any code:
- [ ] Capsule exists in `tasks/` with clear intent and scope
- [ ] `docs/project_state.md` reviewed for current constraints
- [ ] `docs/decisions.md` reviewed for architectural patterns
- [ ] WIP limit check: Are there already 3+ active capsules?

### During Implementation

- [ ] Follow capsule scope (don't modify files outside allowed scope)
- [ ] Write tests as specified in capsule requirements
- [ ] Update `docs/project_state.md` if adding new capabilities
- [ ] Create ADR entry if making architectural decisions

### Pre-Commit Checklist

Before committing:
- [ ] All tests pass
- [ ] Code follows project constraints from `docs/readme.md`
- [ ] `docs/project_state.md` updated with new capabilities/changes
- [ ] Commit message includes capsule reference (T-XXX)
- [ ] Capsule verification checklist completed

### Pre-Completion Checklist

Before marking capsule as complete:
- [ ] All acceptance tests from capsule pass
- [ ] `docs/project_state.md` updated
- [ ] ADR created if needed
- [ ] Capsule moved from Active → Completed in `docs/tasks.md.txt`
- [ ] Manual sanity check performed

## Git Workflow

### Mandatory Commit After Each Task

**CRITICAL RULE:** After completing ANY task from `kanban.md`, you MUST commit immediately with a detailed message. Do not batch multiple tasks into a single commit.

**Commit format** (always reference the task ID):
```bash
git add .
git commit -m "T-XXX: <short summary>

<detailed description of what was done and why>

Changes:
- <specific change 1>
- <specific change 2>

See: kanban.md | tasks/T-XXX-<title>.md (if capsule exists)"
```

**Example:**
```bash
git commit -m "T-001: Define project vision and core documentation

Populated docs/readme.md with mission, scope, constraints, and success criteria.
Established bjarne as an AI-assisted C/C++ validation tool.

Changes:
- Added mission statement focused on code correctness over compilation
- Defined 6-gate validation pipeline scope
- Listed non-goals to prevent scope creep
- Set success criteria with measurable targets

See: kanban.md"
```

### Workflow Integration

1. **Move task to Doing** in `kanban.md`
2. **Complete the work**
3. **Update kanban.md** — move task to Done with completion date
4. **Commit immediately** — detailed message with T-XXX reference
5. **Proceed to next task**

**Never:**
- Skip commits after completing work
- Combine multiple tasks into one commit
- Use vague commit messages like "updates" or "fixes"

**Current branch:** `master`
