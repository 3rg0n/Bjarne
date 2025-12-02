# Capsule T-000 – <Short Title>
<One-sentence intent statement: what this capsule accomplishes and why it matters.>

**Status:** Draft | Planned | In Progress | Completed  
**Estimated Time:** <e.g., 1 hour>  
**Created:** YYYY-MM-DD  
**Dependencies:** <list other capsules or “None”>

---

## 🎯 Intent
<Describe the purpose and expected outcome in 2–3 sentences.>

Example:  
> Implement a `GET /users/:id` API endpoint that returns user data with authentication and error handling.

---

## 📐 Scope
**Allowed edits**
- <specific files or directories the capsule can modify>

**Out of scope**
- <things this capsule must not touch, e.g., renames, architecture changes>

---

## ✅ Acceptance Tests
Checklist of behavior that proves this task is done.

- [ ] <test 1 – expected behavior>
- [ ] <test 2 – error handling case>
- [ ] <test 3 – edge case>
- [ ] <test 4 – performance or limit case>

---

## 🧩 Technical Specification
Describe *how* this should be implemented.

**Files to Create / Modify**
```text
src/
  api/
    users/
      route.ts
tests/
  api/
    users.test.ts
````

**Implementation Steps**

1. <step 1>
2. <step 2>
3. <step 3>

**Constraints**

* Must follow rules from project rules file (`.cursorrules` / `AGENTS.md`).
* Include TypeScript types and tests.
* Handle all expected error states gracefully.

---

## 🧪 Test Requirements

Describe required tests or link to test files.

* Unit tests: <file or framework>
* Integration tests: <file or framework>
* Edge cases to cover: <list>

---

## 🔒 Security / Compliance Considerations

* <e.g., authentication required, no PII logged>
* <e.g., validate input schema with Zod>

---

## 💾 Verification Checklist

Before marking complete, confirm:

* [ ] Code and tests compile successfully
* [ ] All tests pass in CI
* [ ] Manual sanity check passed
* [ ] `PROJECT_STATE.md` updated to reflect capability
* [ ] Capsule status set to **Completed**

---

## 🧠 Notes / Learnings (Optional)

Document discoveries, blockers, or improvements for next time.

---

## 🧾 Completion Actions

When finished:

1. Run tests: `npm test` (or equivalent)
2. Commit:

   ```bash
   git add .
   git commit -m "T-000: <short summary>  
   
   <brief what and why>  
   See: tasks/T-000-<short-title>.md"
   ```
3. Move capsule to **Completed** in `TASKS.md`.

---

> Each capsule should represent ~1–2 hours of focused work with clear boundaries.
> If you can’t finish and test it in a session, split it into smaller capsules.

