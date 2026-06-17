# Feedback note — design

## Problem

There is no way for teams using Daylight to send the maintainer feedback or bug
reports. Daylight runs unattended as a GitLab CI job; if it misbehaves or a user
has a suggestion, there is no channel back to the maintainer.

## Goal

On the first time Daylight processes a merge request, post one short,
human-readable note inviting the team to send feedback via a link to a feedback
form. The note appears exactly once per MR — never on later commits — so it is
useful without being spammy.

## Decisions

- **Channel:** an in-MR note (not email, not a docs link, not a CLI subcommand).
- **Destination:** a hardcoded feedback-form URL the maintainer controls. A
  placeholder const is used until the real form exists.
- **Frequency:** once per MR — only on the first processing run.
- **Visibility:** confidential note for now (visible to project members),
  reusing the existing note mechanism. The code isolates the visibility decision
  so changing it later (e.g. to a public note) is a single-file edit.

## Design

### 1. `internal/pipeline/feedback.go` — owns the feedback note end-to-end

The form URL, the body text, and the visibility all live in this one file, so
any future change (URL, wording, confidential vs public) is a single-file edit.

```go
const feedbackURL = "https://PLACEHOLDER-FORM-URL" // TODO: real form URL

// feedbackNote returns the body and visibility of the one-time feedback note.
func feedbackNote() (body string, confidential bool)
```

The body renders as Markdown, e.g.:

> 🌅 Reviewers on this MR were assigned by **Daylight**. Hit a bug or have
> feedback? [Let us know](https://…).

`confidential` returns `true` for now.

### 2. `internal/gitlab/client.go` — generalize the posting seam

Today `PostInternalNote` hardwires `confidential: true`. Replace it on the
`GitLabClient` interface and on `Client` with:

```go
PostNote(ctx context.Context, projectID, mrIID, body string, confidential bool) error
```

Both notes post through this one method with an explicit visibility flag at the
seam:

- the processed (idempotency) note passes `true`;
- the feedback note passes whatever `feedbackNote()` decided.

### 3. `internal/pipeline/pipeline.go` — post once, best-effort

"First processing of this MR" is already detectable: `prevSHA == ""` means no
prior Daylight note exists. After posting the processed note on that first run,
also post the feedback note.

**Best-effort:** if the feedback post fails, log a warning and continue — it must
never fail the job. The processed note stays fatal, because idempotency depends
on it.

## Error handling & edge cases

- A first run that assigns **no** reviewers returns early (existing behavior)
  before any note is posted — so no lone feedback note. Feedback only rides along
  with a real assignment.
- Subsequent commits (`prevSHA != ""`) skip the feedback note.
- A feedback-post failure logs a warning and the run still succeeds.

## Testing

- `feedback_test.go`: asserts the body contains the form URL and that the
  visibility flag is as expected.
- Pipeline tests:
  - first run (no prior notes) posts exactly one feedback note;
  - second run (prior processed note exists, new SHA) posts none;
  - a feedback-post error still yields a successful run.
- The test fake records `PostNote` calls with the confidential flag.
