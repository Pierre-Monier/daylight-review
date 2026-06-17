# Per-scope reviewer stability — design

## Problem

The reviewer-assignment pipeline churns reviewers when it shouldn't.

Today the idempotency note stores only `sha` and `files_hash`. In `pipeline.Run`:

- If the same commit SHA was already processed → skip.
- If the same `files_hash` was already processed → skip.
- Otherwise → fully recompute: `ResolveAndSelect` picks reviewers fresh for *every*
  matched scope, `SetReviewers` replaces the entire reviewer list, and a new note is posted.

So when a developer edits one more file under a scope (CODEOWNERS section) that was
*already* matched, the file set changes → `files_hash` changes → the skip does not fire
→ the random strategy re-rolls a new reviewer for **every** scope. All reviewers churn,
even scopes whose matched state did not meaningfully change.

## Goal

Make the **scope** the unit of stability, not the file set:

- **Scope present in both the previous and current run** → keep its existing reviewer(s)
  verbatim. No re-roll.
- **Scope newly matched** (a changed file now matches a section that had no matched files
  before) → select reviewer(s) for just that scope.
- **Scope no longer matched** (no changed file matches it anymore) → remove its reviewer(s).

Each run touches only the delta of scopes.

## Decisions (settled during brainstorming)

1. **Source of truth = the stored note only.** We reconcile against what we last assigned,
   recorded in the internal note. We do not read or reconcile against live MR reviewers, so
   manual reviewer edits by humans are ignored.
2. **A scope present in both runs is left fully as-is.** Even if the section's
   `RequiredCount` changed or the matched rule's owner list changed so the stored reviewer
   is no longer technically an owner, the stored reviewer(s) are kept verbatim. (YAGNI — we
   do not reconcile count or pool for present scopes.)
3. **Empty-clear is intended.** If a change removes the last file under every scope, the
   stored reviewers all drop and we call `SetReviewers` with an empty list, actively
   clearing reviewers. This is a behavior change from today (which returns early and leaves
   them) and is the desired behavior.

## State model

The internal note is the single source of truth for what we assigned, per scope.

- Drop `files_hash` — obsolete. "Did anything change?" is now answered by comparing matched
  scopes, not file hashes.
- Keep `sha` — still used for the pure-idempotency skip (same commit reprocessed by a
  pipeline retry → do nothing).
- Add `assignments` — a map of scope name → list of assigned reviewer usernames.

### Encoding: JSON in the note body

CODEOWNERS section names can contain spaces (e.g. `[Backend Team]`), which would break the
current space-delimited `key=value` note format. JSON is robust to this and stays greppable
via the `daylight:processed` prefix:

```
daylight:processed {"sha":"abc123","assignments":{"Backend":["alice","bob"],"Frontend":["carol"]}}
```

## The diff (core logic)

On each run:

1. Read MR notes, take the **most recent** `daylight:processed` note → previous state
   (`sha` + `assignments`). GitLab returns notes newest-first, so the latest note is the
   current state.
2. If `previous.sha == currentSHA` → skip (already processed this exact commit; covers
   pipeline retries).
3. Resolve currently-matched scopes from changed files. `ownership.Resolve` already returns
   `scope → candidate pool`; the set of keys is the currently-matched scopes.
4. Diff stored scopes vs current scopes:
   - **In both** → copy stored reviewer(s) verbatim. No re-roll.
   - **New** (current only) → select reviewer(s) for it, count = section `RequiredCount`,
     excluding the author and anyone already assigned to a kept or other new scope
     (preserves today's cross-scope dedup).
   - **Removed** (stored only) → drop its reviewer(s).
5. New reviewer set = union of all kept + newly-selected reviewers.
6. **Only if the reviewer set differs** from the previous union → call `SetReviewers`. Then
   **always** post an updated note recording the new `sha` + `assignments`, so a retry of
   *this* commit is idempotent even when scopes did not change.

### First run

No prior `daylight:processed` note → every matched scope is "new" → select all. Identical to
today's first-run behavior.

### Exhausted pool for a new scope

If a new scope's candidate pool is exhausted after excluding the author and already-assigned
reviewers, that scope simply receives fewer (or no) reviewers — same as today, where
`strategy.Select` returns an error and the selection round breaks. No crash; the rest of the
diff proceeds.

## Code touch points

- `internal/pipeline/idempotency.go` — replace `FormatNote`/`ParseNote` with a JSON state
  encode/parse (`sha` + `assignments` map). Drop `files_hash`.
- `internal/pipeline/pipeline.go` —
  - `ResolveAndSelect` becomes a scope-diff function: takes previous assignments, current
    candidate pools (with per-scope required counts), author, and strategy → returns the new
    assignments map.
  - `Run` rewires to: read the latest-note state, skip on matching sha, compute the diff,
    call `SetReviewers` only when the union changed, and always post the updated note.
  - `filesHash` is removed.
- The `check` subcommand verbose output adapts to show kept / added / removed per scope.

## Testing

The diff logic is pure (previous assignments + current pools + author + strategy → new
assignments) and is tested directly with a deterministic strategy:

- First run: all scopes new → all selected.
- Re-run, same scopes → assignments identical, no churn.
- Re-run, one scope added → only that scope gets a new reviewer; others unchanged.
- Re-run, one scope removed → that scope's reviewer dropped; others unchanged.
- Re-run, all scopes removed → empty assignments (empty-clear).
- Cross-scope dedup: a newly-added scope whose pool overlaps a kept scope does not reuse the
  already-assigned reviewer when alternatives exist.
- `sha` skip: matching previous sha → no-op.
- JSON note round-trips through `FormatNote`/`ParseNote`, including scope names with spaces
  and empty assignments.
