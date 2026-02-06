# Ralph Autonomous Agent Task

## Overview
You are running as a Ralph autonomous agent. You have permission to execute tasks without user prompts.
Work autonomously, commit frequently, and update your progress.

## Setup (MUST complete before investigating the task)
1. Read `.ralph/config.json` to understand your execution context
2. Read `.ralph/guardrails.md` for learned failure patterns to avoid

## Task Description
Build a Go web app for federal tax withholding estimation with paystub PDF uploads, htmx UI, SQLite persistence, and comprehensive TDD test coverage

## Context
# Task Context

## Goal
Build a personal Go web app for estimating federal tax withholding across two incomes (user and spouse), fed by periodic paystub PDF uploads. The app uses Go + htmx + Go templates for the frontend and SQLite for persistence.

## Tech Stack
- **Language:** Go
- **Web:** Standard library or lightweight framework + htmx + Go templates
- **Database:** SQLite with configurable DB_PATH (default: ./data/tax.db)
- **PDF Parsing:** Go library (text-based PDFs only)
- **Linting:** golangci-lint

## Architecture Notes
- This is a personal tool — no authentication needed
- DB path must be configurable via environment variable (DB_PATH) for future Raspberry Pi deployment
- Use SQLite migrations so the schema is reproducible on any machine
- Tax bracket data should be fetched from IRS sources and cached per year in SQLite
- Paystubs are uploaded every pay period to refine the withholding estimate over time

## Implementation Approach (TDD — module-level)

Work through these in order. For each module, write tests FIRST, then implement until they pass.

### 1. Project scaffolding
- Go module init, directory structure, Makefile
- golangci-lint config
- SQLite setup with migrations, configurable DB_PATH

### 2. Tax bracket data (tests first)
- Fetch current federal brackets and standard deductions from IRS sources
- Cache per year in SQLite
- Fallback to hardcoded 2025/2026 brackets if fetch fails
- Support filing statuses: Single, Married Filing Jointly, Married Filing Separately, Head of Household

### 3. PDF parsing (tests first)
- Upload endpoint accepting PDF (and other common formats if low lift)
- Extract: first name, gross pay, federal tax withheld, pay period dates, YTD totals
- Strip ALL sensitive data (SSN, address, bank account numbers, employer ID, etc.)
- Only retain first name as identifier
- Clear error messages for unparseable/image-based PDFs

### 4. Data persistence (tests first)
- Store paystub records by person (first name) and tax year
- Store filing status per tax year
- Cache tax bracket data by year
- Handle duplicate paystub uploads gracefully (same person, same pay period)

### 5. Withholding calculator (tests first)
- Calculate combined tax liability from two incomes using cached brackets
- Apply standard deduction based on filing status
- Subtract total withheld-to-date from both earners
- Recommend additional withholding for the higher earner
- Recalculate as more pay periods are uploaded
- Determine remaining pay periods in the year to spread additional withholding

### 6. Web UI (htmx + Go templates)
- Upload page for paystubs
- Filing status configuration per tax year
- Dashboard with withholding recommendation
- Visualizations: tax bracket explorer, withholding trend over time
- Prompt for supplemental income (interest, etc.) with materiality threshold — only ask when it would significantly affect the calculation

## Completion Criteria

### P0: Must pass (automated checks)
- [ ] `go build ./...` exits 0
- [ ] `go test ./...` passes
- [ ] `golangci-lint run` passes

### P1: Test coverage — PDF Parsing
- [ ] Extracts gross pay, federal tax withheld, pay period dates from sample fixture
- [ ] Extracts YTD totals (gross, federal withheld)
- [ ] Returns clear error for non-PDF file upload
- [ ] Returns clear error for image-based/unparseable PDF
- [ ] Strips sensitive data — only first name retained

### P1: Test coverage — Tax Bracket Logic
- [ ] Correct tax calculation for Single filer at multiple income levels
- [ ] Correct tax calculation for Married Filing Jointly
- [ ] Handles bracket boundaries (income exactly at bracket edge)
- [ ] Standard deduction applied correctly per filing status

### P1: Test coverage — Withholding Recommendation
- [ ] Calculates combined tax liability from two incomes
- [ ] Subtracts total withheld-to-date from both earners
- [ ] Recommends additional withholding for the higher earner
- [ ] Adjusts recommendation as more pay periods are uploaded

### P1: Test coverage — API/Handlers
- [ ] Upload endpoint accepts PDF and returns extracted data
- [ ] Upload endpoint rejects invalid file types
- [ ] Filing status can be set and persisted
- [ ] Tax bracket data cached per year in SQLite

### P1: Test coverage — Data Storage (SQLite)
- [ ] Paystub record saved and retrieved correctly
- [ ] Paystubs queryable by person (first name) and tax year
- [ ] Filing status saved and retrieved per tax year
- [ ] Tax bracket cache stored and retrieved by year
- [ ] Duplicate paystub upload (same person, same pay period) handled gracefully

### P2: Must verify (user, after delivery)
- [ ] Upload actual paystub — fields extracted correctly
- [ ] Withholding recommendation makes sense for real situation
- [ ] Visualizations render in browser

### Out of scope (do NOT do these)
- No state tax calculations (federal only)
- No user authentication or multi-user accounts
- No deployment setup (Raspberry Pi, cloud, Docker compose, etc.)
- No frontend JavaScript framework — Go templates + htmx only
- No OCR or image-based PDF support
- Do not hardcode the DB path
- Do not store sensitive PII (SSN, address, bank accounts) in the database
- Do not refactor or over-engineer — keep it simple and functional

## Error Handling
- Tests fail: Fix and retry
- PDF parsing fails on a fixture: Adjust parser and re-run tests
- IRS tax data fetch fails: Fall back to hardcoded brackets for current year
- Unclear requirement: Make a reasonable judgment call and document the decision in DECISIONS.md at the repo root

## Additional Notes
- The user is learning Go through this project — write idiomatic Go with clear patterns
- Use Go's standard library where possible before reaching for third-party packages
- The user plans to upload paystubs every pay period, so the UX for repeated uploads should be smooth
- Visualizations should be explorable — the user wants to understand their tax bracket, not just see a number


## GitHub Authentication

You are running in a Docker sandbox. GitHub authentication setup:

**First, export the token** (required before any gh/git operations):
```bash
export GH_TOKEN=$(cat /run/secrets/gh_token)
gh auth setup-git  # configures git credential helper
```

After this:
- `gh` CLI will work for all GitHub operations
- Git push/pull will use the credential helper
- Git identity is pre-configured as "Claude (Ralph Agent)" <noreply@anthropic.com>

**If you encounter permission denied errors:**
1. This is a token permissions issue - you cannot fix it autonomously
2. Document the error in `.ralph/progress.md`
3. Note the specific permission needed (e.g., `actions:write`, `secrets`)
4. Stop and report - the orchestrator will need to update the token

**Token scope:** This agent is authorized for `brendanwhit/tax-withholding-estimator` only.
Tokens are managed by the orchestrator using `ralph-token` CLI.
If you need access to other repos, that requires orchestrator intervention.

## Ralph Protocol

### Progress Tracking
- Update `.ralph/progress.md` after completing each significant step
- Use Beads (`bd`) to track task status if available:
  - `bd ready` to see available tasks
  - `bd start <id>` to claim a task
  - `bd done <id>` to mark complete

### Git Workflow
- Commit frequently with descriptive messages
- Each commit should represent a logical unit of work
- Push to remote when you have stable progress

### Guardrails
- Before attempting risky operations, check `.ralph/guardrails.md`
- If you encounter a failure, add it to guardrails for future reference
- Learn from past mistakes to avoid repeating them

### Completion
- When task is complete, update progress.md with final status
- Create a PR if appropriate (use `gh pr create`)
- Mark Beads task as done if used
