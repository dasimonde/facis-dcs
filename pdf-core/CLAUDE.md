# No-Bullshitting rule (Mandatory)

This is a green field project. Never introduce "fallback" or "legacy" compatibility artifacts. Prune dead code.

# Test-First Development (Mandatory)

The system uses both behavior-driven development tests by gherkin feature files for use-case, and go-unit tests.

All changes must follow Test-First Development.

Before writing or modifying production code:

1. Write a failing test.
2. Verify the test fails for the expected reason.
3. Implement the solution.
4. Verify the test passes.
5. Refactor while keeping tests green.

Never write production code before the test that proves it is needed.

---

## No Exceptions

This applies to:

* bug fixes
* new features
* refactors
* performance improvements
* infrastructure changes
* configuration changes
* database changes
* API changes

If a change cannot be tested, explain why explicitly before making it.

---

## Bug Fix Procedure

When fixing a bug:

1. Create a test that reproduces the bug.
2. Confirm the test fails.
3. Implement the fix.
4. Confirm the test passes.
5. Ensure related tests still pass.

A bug is not considered fixed until a test exists that would catch its reintroduction.

---

## Refactoring Procedure

Before refactoring:

* ensure existing behavior is covered by tests

If coverage is insufficient:

* add tests first

Never refactor untested behavior.

---

## No Manual-Testing-Only Development

Manual testing is supplementary.

Manual verification does not replace automated tests.

Claims such as:

* "it should work"
* "it compiles"
* "it looks correct"
* "I tested it manually"

are not evidence of correctness.

Tests are the evidence.

---

## Test Quality Requirements

Tests must:

* verify observable behavior
* verify real requirements
* fail when the implementation is broken
* be deterministic
* be maintainable

Avoid tests that merely exercise code without asserting outcomes.

---

## Completion Criteria

A task is not complete unless:

* tests exist
* tests fail before implementation
* tests pass after implementation
* all relevant test suites pass

Code without tests is incomplete work.

---

## Build and Test Commands

Use `make` targets, not raw `go` commands directly.

| Goal | Command |
|------|---------|
| Regenerate Goa transport | `make gen` |
| Build the binary | `make build` |
| Run Go unit/integration tests | `make test` |
| Run BDD scenarios (needs server) | `make bdd` |
| Run everything | `make test-all` |

**When to run `make gen`:** Goa transport is regenerated automatically on every `make test`, `make build`, and `make bdd`, so you rarely need to run it directly. Run `make gen` on its own only when you want to regenerate without building or testing.

**`make test` vs `make bdd`:** prefer `make test` for fast feedback during development. Run `make bdd` when the change touches HTTP transport, request/response encoding, or end-to-end behaviour. Run `make test-all` before considering a task complete.

**Never run `go test ./...` without `make gen` having run first** — the generated transport in `gen/` must exist or the build fails.
