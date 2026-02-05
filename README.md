# New Project

## Goal
Create new GitHub repo brendanwhit/tax-withholding-estimator with README.md - testing default token fallback

## Context
# Task Context

## Goal
Create a new GitHub repository `brendanwhit/tax-withholding-estimator` with a README.md file. This is a minimal test to verify the default token fallback mechanism works correctly in Docker sandbox mode.

## What To Do

1. Create the GitHub repository:
   ```bash
   gh repo create brendanwhit/tax-withholding-estimator --public --description "Tax withholding estimator"
   ```

2. Clone or initialize the repo locally

3. Create README.md with content like:
   ```markdown
   # Tax Withholding Estimator

   A tool to estimate tax withholding amounts.

   ## Overview

   This project will help estimate federal and state tax withholdings based on income, filing status, and other factors.

   ## Status

   🚧 Under construction - initial project setup.
   ```

4. Commit and push to main branch

5. Verify the repo is accessible on GitHub

## Completion Criteria

### Must pass (automated checks)
- [ ] `gh repo view brendanwhit/tax-withholding-estimator` succeeds (repo exists)
- [ ] `gh api repos/brendanwhit/tax-withholding-estimator/contents/README.md` returns 200 (README exists)

### Must verify (inspect by reading)
- [ ] README.md contains "Tax Withholding Estimator" title
- [ ] README.md has a brief project description

### Out of scope (do NOT do these)
- Do NOT create any code files (no Python, JS, Go, etc.) - ONLY README.md
- Do NOT set up CI/CD, workflows, or GitHub Actions
- Do NOT add license files, .gitignore, or other boilerplate
- Do NOT create issues or project boards
- Do NOT invite collaborators or change repo settings beyond creation
- Do NOT work on any other repositories - ONLY brendanwhit/tax-withholding-estimator

## Error Handling

If you encounter errors:
1. **Try to resolve them** - attempt reasonable fixes
2. **Document all friction** - print a detailed report of:
   - What error occurred
   - What you tried to fix it
   - Whether the fix worked
   - Any unexpected behavior or warnings

This task is specifically testing the default token fallback mechanism, so detailed reporting of any authentication or permission issues is especially important.

## Token Context

There is NO specific token for `brendanwhit/tax-withholding-estimator`. The system should fall back to the `brendanwhit/_default` token. If authentication fails, document exactly what happened.

## Additional Notes

- This is a greenfield project - the repo does not exist yet
- Keep it minimal - the goal is to test infrastructure, not build a real app
- Success = repo exists with README.md, completed without user interaction


## Getting Started

This project was scaffolded by Ralph agent.
