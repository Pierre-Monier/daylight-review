# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Daylight is a Go CLI that auto-assigns reviewers to GitLab merge requests based on a
`CODEOWNERS` file. It runs as a GitLab CI job on `merge_request_event` — there is no server.
The binary reads everything it needs from CI environment variables, makes a handful of GitLab
REST calls, and exits.

## Commands

```bash
go build ./cmd/daylight        # build the CLI
go test ./...                  # run all unit tests
go test ./internal/pipeline    # test a single package
go test -run TestName ./internal/ownership   # run a single test

# local dry-run of CODEOWNERS resolution — no API calls
daylight check -codeowners CODEOWNERS -author alice path/to/changed.go ...

# build & push the multi-arch Docker image to Docker Hub (daylightreview/daylight)
./scripts/push-image.sh [version]
```

The GitLab client test (`internal/gitlab/client_test.go`) is an integration test that hits a
real GitLab instance. It is skipped unless `DAYLIGHT_GITLAB_TOKEN`, `DAYLIGHT_GITLAB_URL`,
`TEST_PROJECT_ID`, and `TEST_MR_IID` are all set. Everything else is a pure unit test with no
network.

## Architecture

The flow is a single pass orchestrated by `pipeline.Run` (`internal/pipeline/pipeline.go`):

1. **config** — `config.Load()` reads/validates CI env vars (`CI_PROJECT_ID`,
   `CI_MERGE_REQUEST_IID`, `CI_COMMIT_SHA`, `DAYLIGHT_GITLAB_TOKEN`; optional
   `DAYLIGHT_GITLAB_URL`, falling back to `CI_SERVER_URL` then `gitlab.com`).
2. **gitlab** — `GitLabClient` is an interface over the five REST operations the pipeline
   needs (`MRNotes`, `MRChanges`, `CODEOWNERSContent`, `SetReviewers`, `PostNote`). The
   pipeline depends on the interface so it can be tested with a mock; `*Client` is the real
   HTTP implementation. `SetReviewers` takes usernames and resolves each to a user ID via the
   GitLab users API before the PUT.
3. **ownership** — `Parse` turns CODEOWNERS text into `[]Section` (each with rules + a
   required reviewer count from the `[Name][N]` header). `Resolve` prefix-matches changed
   files to sections, producing a `section → candidate usernames` map. `NormalizePath`
   prepends `/` because GitLab CI omits the leading slash from changed paths.
4. **selection** — `SelectionStrategy.Select(candidates, exclude)` picks one reviewer.
   `RandomStrategy` is the only implementation; the interface exists so the pipeline takes a
   strategy as a parameter.
5. **pipeline** — `DiffAndSelect` is the core logic and is shared by both `Run` and the
   `check` subcommand (via the `printf` callback so it can log to stdout or the std logger).

### Idempotency & per-scope stability (the subtle part)

After each run Daylight posts a **confidential** internal note whose body is
`daylight:processed ` + JSON `{sha, assignments}` (`internal/pipeline/idempotency.go`). On the
next run it reads notes newest-first, parses the first daylight note as the previous state, and:

- If the stored SHA equals the current `CI_COMMIT_SHA`, it skips entirely (already processed).
- Otherwise `DiffAndSelect` diffs the currently-matched scopes against the stored assignments:
  a scope present in both **keeps its stored reviewers verbatim** (stability — reviewers don't
  churn when unrelated files change), a newly-matched scope gets fresh reviewers (excluding the
  author and anyone already assigned), and a no-longer-matched scope is dropped.

This per-scope diffing is why the note stores a `section → reviewers` map rather than a flat
list. When changing assignment behavior, preserve this stability invariant.

A reviewer can be flagged out-of-office with an MR comment `daylight:ooo @user`. On the
next run (push or job retry) Daylight reads the comment from the notes it already fetches,
drops that reviewer from their scope, and backfills a replacement (excluding the author,
out-of-office users, and already-assigned reviewers). The flagged set is stored in the
processed note (`ooo`) and is sticky for the MR — a flagged user is never re-selected. A
new OOO command overrides the same-SHA skip. If a scope has no available replacement it is
left short and a one-time non-confidential warning note is posted.

Known limitation, documented in code: `MRNotes` fetches only the first page (100 notes, no
pagination), so on a very chatty MR the idempotency note could fall off page 1.

## Design docs — read these before changing behavior

The `docs/` tree is the canonical source for *why* the code behaves as it does. The
implemented code is only the "assignment" slice of a broader product.

- **`docs/product/product-vision.md`** (French) — the full product vision. Two pillars:
  *Équité* (automatic, deterministic reviewer assignment — what's built) and *Visibilité*
  (cross-team review/load visibility — not built). Useful for understanding intent and what's
  deliberately out of scope.
- **`docs/us/us1.md`–`us4.md`** (French) — the MVP user stories with Gherkin acceptance
  criteria, mapping 1:1 to the pipeline: US-1 detect MR / draft-skip / idempotency, US-2
  resolve ownership from CODEOWNERS, US-3 select one reviewer per team (random, exclude
  author, dedup), US-4 apply assignment with **no churn** + completion-only on new scopes.
  These criteria are the spec for assignment behavior.
- **`docs/superpowers/specs/`** (English) — dated design docs. `tech-stack-design.md` is the
  MVP architecture rationale (note: it predates two changes — idempotency no longer uses a
  `files_hash`, and `PostInternalNote` was renamed to `PostNote` with an explicit
  `confidential` flag). `per-scope-reviewer-stability-design.md` is the authoritative
  explanation of the scope-diff logic, including settled decisions: the **stored note is the
  only source of truth** (manual reviewer edits by humans are ignored), present scopes are
  kept verbatim even if count/owners changed (YAGNI), and empty-clear is intentional.
  `feedback-note-design.md` covers the one-time feedback note.
- **`docs/superpowers/plans/`** (English) — dated step-by-step implementation plans
  corresponding to the specs.

When acceptance criteria in the user stories or a spec conflict with what the code does, treat
the code as current and the docs as intent — and flag the divergence.

## Work in progress

The `feature/feedback-note` branch is mid-TDD: `internal/pipeline/feedback_test.go` exists and
fails because `internal/pipeline/feedback.go` (`feedbackNote()` + `feedbackURL`) is not written
yet, and the one-time post is not yet wired into `pipeline.Run`. See
`docs/superpowers/specs/2026-06-17-feedback-note-design.md` and the matching plan.

## Conventions

- Every source file starts with two `// ABOUTME:` comment lines describing what it does.
- The project follows TDD: unit tests sit beside each package and cover the real logic, not
  mocks. The pipeline's mock is only for injecting GitLab responses.
