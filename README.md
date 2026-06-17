# Daylight Review

Automatically assigns reviewers to GitLab merge requests based on `CODEOWNERS` ownership. Runs as a GitLab CI job — no server to operate.

## How it works

On every MR event, Daylight reads your `CODEOWNERS` file, finds which teams own the changed files, picks one reviewer per team at random (excluding the MR author), and assigns them. It posts a confidential internal note after each run so it never assigns twice for the same changeset.

## Quick start

### 1. Add a `CODEOWNERS` file to your repository

Two supported formats:

**Owners on the rule line:**
```
[Backend][1]
/src/ @alice @bob
/api/ @carol
```

**Default owners on the section header:**
```
[Backend][1] @alice @bob
/src/
/api/
```

### 2. Create a GitLab access token

In GitLab, go to **User Settings → Access Tokens** (or use a project/group access token) and create a token with the `api` scope.

### 3. Add the token as a CI/CD variable

In your project or group: **Settings → CI/CD → Variables**

| Name | Value | Protected | Masked |
|---|---|---|---|
| `DAYLIGHT_GITLAB_TOKEN` | your token | ✓ | ✓ |

### 4. Add the job to `.gitlab-ci.yml`

```yaml
daylight-assign:
  image: daylightreview/daylight:latest
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  script:
    - daylight
```

That's it. Daylight reads all other required values (`CI_PROJECT_ID`, `CI_MERGE_REQUEST_IID`, `CI_COMMIT_SHA`, `CI_SERVER_URL`) directly from the GitLab CI environment.

## Configuration

| Variable | Required | Description |
|---|---|---|
| `DAYLIGHT_GITLAB_TOKEN` | ✓ | GitLab API token with `api` scope |
| `DAYLIGHT_GITLAB_URL` | — | Override the GitLab instance URL (defaults to `CI_SERVER_URL`, then `https://gitlab.com`) |

## Behaviour

- **Draft MRs are skipped** — no API call is made, the job exits immediately.
- **Idempotent** — if the changeset hasn't changed since the last run, no assignment is made.
- **No owner** — if none of the changed files have an owner in `CODEOWNERS`, the job exits silently without assigning anyone.
- **Author excluded** — the MR author is never selected as a reviewer.
- **One reviewer per team** — if the same person is a candidate for multiple teams, they are assigned only once.

## Self-hosted GitLab

Set `DAYLIGHT_GITLAB_URL` to your instance URL:

```yaml
daylight-assign:
  image: daylightreview/daylight:latest
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  variables:
    DAYLIGHT_GITLAB_URL: https://gitlab.yourcompany.com
  script:
    - daylight
```

## Full pipeline example

If you also want to publish the image from your own fork:

```yaml
stages:
  - publish
  - assign

publish-image:
  stage: publish
  image: docker:27
  services:
    - docker:27-dind
  rules:
    - if: $CI_COMMIT_BRANCH == $CI_DEFAULT_BRANCH
  script:
    - echo "$DOCKER_HUB_TOKEN" | docker login -u "$DOCKER_HUB_USERNAME" --password-stdin
    - docker build -t yourorg/daylight:latest -t yourorg/daylight:$CI_COMMIT_SHORT_SHA .
    - docker push yourorg/daylight:latest
    - docker push yourorg/daylight:$CI_COMMIT_SHORT_SHA

daylight-assign:
  stage: assign
  image: yourorg/daylight:latest
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  script:
    - daylight
```
