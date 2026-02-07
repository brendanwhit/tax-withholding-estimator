# Beads Issue Tracking

This project uses **beads** (`bd` CLI) for issue tracking. Do NOT use TodoWrite, TaskCreate, or markdown files for tracking work.

## Quick Reference

### Finding Work
- `bd ready` - Show issues ready to work (no blockers)
- `bd list --status=open` - All open issues
- `bd show <id>` - Issue details with dependencies

### Workflow
1. `bd create --title="..." --type=task|bug|feature --priority=2` - Create issue before starting work
2. `bd update <id> --status=in_progress` - Mark as in progress when starting
3. `bd close <id>` - Mark complete when done
4. `bd sync --from-main` - Sync before committing

### Priority Scale
- P0 = critical, P1 = high, P2 = medium (default), P3 = low, P4 = backlog
- Use numbers 0-4, NOT "high"/"medium"/"low"

### Session Close Protocol
Before saying "done", run:
```bash
git status
git add <files>
bd sync --from-main
git commit -m "..."
```

### Dependencies
- `bd dep add <issue> <depends-on>` - Add dependency
- `bd blocked` - Show blocked issues

### Tips
- Close multiple issues at once: `bd close <id1> <id2> ...`
- Do NOT use `bd edit` - it opens $EDITOR which blocks agents
- Run `bd prime` for full AI context if needed
