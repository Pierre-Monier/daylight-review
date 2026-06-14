# Tech Stack Design — Daylight Review MVP

## Context

Daylight Review automates reviewer assignment on GitLab MRs. The MVP covers four user stories
(US-1 to US-4): detect a MR, resolve ownership from CODEOWNERS, select reviewers, assign them.

## Deployment Model

Ships as a single Go binary / minimal Docker image. Runs as a **GitLab CI job**, not a web
server. GitLab handles event triggering; no infrastructure to operate. Works identically for
SaaS (hosted by us) and self-hosted (deployed by the customer). Deployment model is deferred —
the binary fits both without changes.

Customer setup:
```yaml
# .gitlab-ci.yml or group-level shared template
daylight-assign:
  image: ghcr.io/yourorg/daylight:latest
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  script:
    - daylight assign
```

## Language & Runtime

**Go.** Single binary, no runtime to install, ~10 MB Docker image. Excellent fit for a
stateless CLI that makes HTTP calls and parses text. Strong typing, table-driven TDD, clean
interface story.

## Configuration

All inputs come from CI environment variables — no config file needed.

| Variable | Source | Purpose |
|---|---|---|
| `CI_PROJECT_ID` | GitLab CI | Project identifier |
| `CI_MERGE_REQUEST_IID` | GitLab CI | MR identifier |
| `CI_COMMIT_SHA` | GitLab CI | Current commit SHA |
| `CI_MERGE_REQUEST_DRAFT` | GitLab CI | Draft status |
| `CI_SERVER_URL` | GitLab CI | GitLab instance URL |
| `DAYLIGHT_GITLAB_TOKEN` | CI secret variable | API authentication |
| `DAYLIGHT_GITLAB_URL` | CI secret variable (optional) | Override `CI_SERVER_URL` |

## Idempotency — Stateless via GitLab

No database. State is stored as **internal MR notes** posted by Daylight itself:

```
daylight:processed sha=abc123 files_hash=def456
```

On each run, Daylight fetches existing MR notes and checks:
- `sha` matches `CI_COMMIT_SHA` → already processed for this commit, exit 0
- `files_hash` matches current changed files hash → scope unchanged, exit 0

`files_hash` = SHA-256 of the sorted, newline-joined list of changed file paths (deterministic,
independent of order returned by the API).

Internal notes are visible only to project members and serve as an audit trail.

## Pipeline

Steps execute in this order, cheapest exits first:

```
1. CI_MERGE_REQUEST_DRAFT == "true"  → exit 0          (free, no API call)
2. Fetch MR notes → sha match        → exit 0
3. Fetch MR changed files → compute files_hash
4. Latest daylight note → files_hash match → exit 0
5. Fetch CODEOWNERS at current SHA
6. Parse CODEOWNERS → map changed files to {team → []username}
7. Select one reviewer per team (exclude MR author), dedup across teams
8. PUT MR reviewers via GitLab API
9. POST internal note: daylight:processed sha={SHA} files_hash={hash}
```

Total API calls on the happy path: 5.

## CODEOWNERS Parsing

Two supported patterns. Owners for a rule = explicit owners on the rule line, falling back to
the section header's default owners.

**Pattern A — owners on rule line:**
```
[Panko][1]
/packages/modules/mycharge/ @jean.creuze @kevin.cardon @sara.terrier
```

**Pattern B — default owners on section header:**
```
[Panko][1] @jean.creuze @kevin.cardon @sara.terrier
/packages/modules/mycharge/
```

No GitLab group API call needed — all candidate usernames are read directly from CODEOWNERS.

## Package Layout

```
cmd/daylight/main.go       — entry point, wires config, runs pipeline
internal/
  config/                  — reads and validates CI env vars
  gitlab/                  — API client (notes, changes, CODEOWNERS, set reviewers, post note)
  ownership/               — CODEOWNERS parser + file→team resolver
  selection/               — SelectionStrategy interface + RandomStrategy
  pipeline/                — orchestrates steps 1–9
```

## Key Interfaces

**SelectionStrategy** — seam for future strategies (least-loaded, round-robin):
```go
type SelectionStrategy interface {
    Select(candidates []string, exclude string) (string, error)
}
```

**GitLabClient** — mockable in pipeline tests:
```go
type GitLabClient interface {
    MRNotes(ctx, projectID, mrIID) ([]Note, error)
    // Returns changed file paths and MR author username (same API call).
    MRChanges(ctx, projectID, mrIID) (files []string, authorUsername string, err error)
    CODEOWNERSContent(ctx, projectID, ref) (string, error)
    SetReviewers(ctx, projectID, mrIID, usernames []string) error
    PostInternalNote(ctx, projectID, mrIID, body string) error
}
```

All other types (`ownership.Parser`, `pipeline.Run`) are concrete — no interface unless there
is an active reason to abstract.

## Testing Strategy

**Unit tests (no network):**
- `ownership.Parser` — table-driven: both CODEOWNERS patterns, multi-team files, no-owner
  files, section default fallback
- `selection.RandomStrategy` — candidate pool, author exclusion, empty/single pool
- `pipeline` — mock `GitLabClient`; one test per exit condition + happy path

**Integration tests (real GitLab):**
- `gitlab.Client` — skipped unless `DAYLIGHT_GITLAB_TOKEN` + `DAYLIGHT_GITLAB_URL` are set

**TDD order:** parser → strategy → pipeline steps top to bottom.

No end-to-end test for MVP; the CI job is the end-to-end test.

## Out of Scope (MVP)

- Teams notification (US-5, future)
- Non-random selection strategies
- Cross-project CODEOWNERS references
- Approval rules (distinct from reviewers in GitLab)
