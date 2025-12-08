# What to Work on Next

Help the user identify what to focus on next based on the project's contribution process.

## Contribution Process Reference
@CONTRIBUTING.md

## Instructions

Follow these steps to gather information and present suggestions:

### Step 1: Get GitHub username
Run `gh api user --jq '.login'` to get the current user's GitHub username.

### Step 2: Check for PRs needing review
Run `gh pr list --state open --json number,title,author,isDraft,reviewRequests,reviews,assignees,updatedAt,url` to get all open PRs.

Filter the results:
- **Exclude drafts** unless the current user is an assignee AND there are comments in the last 48 hours
- **Exclude PRs authored by the current user** (you review others' work, not your own)
- **Exclude PRs that are sufficiently reviewed**: A PR is "sufficiently reviewed" if it has at least one APPROVED review, OR has 2+ reviews submitted in the last 24 hours
- **Prioritize PRs** where the current user is explicitly requested as a reviewer

### Step 3: Check for the current user's PRs
From the same PR data, identify:

**Assigned PRs with activity**: PRs where current user is an assignee with recent comments (check with `gh pr view <number> --json comments` if needed). These may need attention even if drafts.

**Your open PRs (non-draft)**: PRs authored by the current user that are not drafts. Note their review status:
- Has approval and no blocking reviews = ready to merge
- Has CHANGES_REQUESTED = needs your attention
- Waiting for review = note who's been requested

**Your drafts**: Draft PRs authored by the current user. Note how recently they were updated.

### Step 4: Find upcoming milestone and priority issues
Run `gh api repos/:owner/:repo/milestones --jq '[.[] | select(.state == "open") | select(.due_on)] | sort_by(.due_on) | .[0] | {number, title, due_on}'` to find the next milestone with a due date (`:owner/:repo` is auto-substituted by gh CLI).

Then fetch issues for that milestone:
```
gh issue list --milestone "<milestone_title>" --state open --json number,title,labels,assignees,url
```

Sort issues by priority (use labels):
1. `priority/critical` - highest
2. `priority/high`
3. `priority/normal` (or no priority label)
4. `priority/low`

Select up to 3 issues, preferring unassigned ones or ones assigned to the current user.

### Step 5: Present suggestions

Format the output as a prioritized list:

```
## What to Focus on Next

### PRs Needing Review (do these first)
1. [PR #123](url) - "Title" by @author
   - Review requested / Needs first review / etc.

### Your Assigned PRs with Activity
1. [PR #456](url) - "Title" (draft)
   - Recent comment from @someone needs response

### Your Open PRs (non-draft)
1. [PR #789](url) - "Title"
   - Status: Waiting for review from @reviewer / Has approval, ready to merge / etc.

### Your Drafts
1. [PR #790](url) - "Title"
   - Last updated X days ago

### Priority Issues for [Milestone Name]
1. [#789](url) - "Issue title" [priority/high]
2. [#790](url) - "Another issue" [priority/normal]
3. [#791](url) - "Third issue" [priority/normal]

---
*Based on the contribution process above: PRs take precedence, then milestone issues by priority*
```

If any section is empty, note that (e.g., "No PRs currently need your review").

### Determining "sufficient review" heuristics
- Has 1+ APPROVED review AND no CHANGES_REQUESTED = sufficiently reviewed
- Has CHANGES_REQUESTED = needs author attention, not reviewer attention (exclude from review list)
- Has 2+ COMMENT reviews in last 24h = active discussion, may not need more reviewers
- Review was requested from specific teams/users and those reviews exist = sufficiently reviewed
