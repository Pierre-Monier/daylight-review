# Out-of-office reviewer reassignment

Date: 2026-08-17
Status: Approved, pending implementation

## Problem

Daylight assigns a reviewer per scope, then keeps that reviewer verbatim across runs
(the per-scope stability invariant). When an assigned reviewer is out of the office,
there is no way to hand their review to someone else — the reviewer stays assigned and
the MR stalls.

No external source of truth for availability can be trusted to be current (GitLab busy
status, HR/leave systems, calendars). The developer working the MR knows who is out.
So the signal must be human-driven, and it must work within Daylight's constraints:
the tool only runs as a GitLab CI job on `merge_request_event` pipelines, reads what it
needs from the MR via REST, and exits. There is no server and no webhook handling.

## Trigger model

Daylight cannot react to a comment, a label change, or a reviewer edit — none of those
fire a `merge_request_event` pipeline. Those pipelines fire on **push (new commit)** and
on **retrying the Daylight job**.

Daylight already fetches all MR notes every run (for idempotency). A comment is durable
state in that notes list. So the flow is:

1. A human posts a plain MR comment: `daylight:ooo @alice`.
2. The Daylight job runs again — next push, or the developer clicks **Retry** on the job.
3. On that run Daylight reads the notes, sees the command, drops `alice` from her scope,
   and selects a replacement.

We do not react to the comment live; we read it the next time the job runs for any reason.

### Command syntax

`daylight:ooo @alice`

- Uses the existing `daylight:` namespace (`daylight:processed` is already used for the
  internal note). A `/`-prefixed quick action was rejected: GitLab intercepts unknown
  `/commands` and would strip or error on them.
- The `@` is optional in parsing. The token after it is a GitLab username.
- One username per command. Multiple people out → multiple comments.
- The comment is a normal, **non-confidential** note so the @-mention notifies the person
  and the reassignment is auditable.

## Idempotency override

Today `pipeline.Run` skips the whole run when `prevSHA == CI_COMMIT_SHA`
("already processed this commit").

New rule: parse OOO commands from the notes on every run. If any OOO'd username is **not
already in the stored OOO list**, there is unprocessed work, so do **not** skip — even at
the same SHA. This is what makes "post comment, retry job, no new commit" reassign.

When there is no new OOO command and the SHA matches, the run still skips as before.

## State: OOO list in the note (sticky)

The internal note JSON extends from `{sha, assignments}` to `{sha, assignments, ooo}`:

```json
{"sha": "...", "assignments": {"scope": ["bob"]}, "ooo": ["alice"]}
```

- `ooo` is the accumulated set of usernames flagged out on this MR.
- It is additive: notes written before this feature have no `ooo` field and parse to an
  empty set. No compatibility shim, no versioning.
- Once a username is in `ooo`, it is **excluded from all selection** for the rest of the
  MR's life — including scopes that appear later. This is the "sticky" behavior: a person
  flagged out is not re-picked while the MR is open.

There is no "un-OOO" command. If someone comes back, a new push naturally keeps the
already-chosen replacement (stability invariant); the flagged person simply isn't
re-selected. Removing someone from `ooo` is out of scope (YAGNI).

## Selection changes (`DiffAndSelect`)

Compute the effective OOO set = stored `ooo` ∪ usernames parsed from OOO commands in the
notes. Then, within the existing scope diff:

- **Kept scope (present in both current pools and stored assignments):** filter out any
  stored reviewer who is in the OOO set. For each reviewer removed this way, backfill one
  fresh reviewer for that scope, excluding OOO + author + already-assigned. Reviewers who
  are not OOO stay exactly where they were — the no-churn invariant holds for everyone
  except the person who is out.
- **New scope:** select as today, with the OOO set added to the exclusions.
- **Dropped scope:** unchanged (a scope no longer matched by changed files is removed).

The OOO set is added to the note written at the end of the run.

### No eligible replacement

If, after excluding OOO + author + already-assigned, the pool for a scope is exhausted,
the OOO'd reviewer is dropped and the scope is left without a reviewer (this is the
existing `selectN` behavior — return fewer reviewers). Daylight records a warning for that
scope. The job still exits 0 and every other scope assigns normally.

## No-replacement warning note

For each scope where an OOO removal could not be backfilled, post a **non-confidential**
comment:

```
⚠️ @alice is out and no other owner of [scope] is available — please assign manually.
```

To avoid re-posting on every push, the warning carries a stable marker (per scope + user)
and is skipped if an identical warning already exists among the MR notes — the same
"post once" mechanism the feedback note uses.

## Testing

Pure unit tests in `internal/pipeline`, driving `DiffAndSelect` and the note round-trip
through the existing mock GitLab client. No network. TDD — tests first.

Cases:

- Parse `daylight:ooo @alice` (and without `@`) from a note body; reject non-OOO notes.
- Note format round-trips `{sha, assignments, ooo}`; a legacy note with no `ooo` field
  parses to an empty OOO set.
- Reassign on command: alice assigned, `daylight:ooo @alice` present, replacement selected
  for her scope excluding alice + author + already-assigned.
- Sticky across scopes: once flagged, alice is not selected for a later new scope.
- Same-SHA override: `prevSHA == CI_COMMIT_SHA` but a new OOO command → run does not skip.
- No eligible replacement: alice is the last owner of her scope → scope left unassigned,
  warning recorded, other scopes unaffected, exit 0.
- Warning posted once: an identical warning already on the MR is not re-posted.

## Out of scope

- Reading any external availability source (GitLab busy status, calendars, HR).
- An "un-OOO" / return-from-leave command.
- Scheduled-pipeline polling of open MRs (would remove the manual retry, but turns Daylight
  into a polling server — a separate project if the retry friction ever justifies it).
