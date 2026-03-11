# Architecture Diagram Sources

This folder contains diagram source scripts for the architecture views.

## Files

- `system_context.py`
- `container_view.py`
- `deployment_view.py`

## Generate diagrams

Use the portable `architecture-diagrams` skill tooling you shared.

From the repository root:

```bash
SKILL_DIR=~/.claude/skills/architecture-diagrams
source "$SKILL_DIR/.venv/bin/activate"
python "$SKILL_DIR/scripts/generate.py" docs/architecture/diagrams/system_context.py --name system-context
python "$SKILL_DIR/scripts/generate.py" docs/architecture/diagrams/container_view.py --name container-view
python "$SKILL_DIR/scripts/generate.py" docs/architecture/diagrams/deployment_view.py --name deployment-view
```

By default, output goes to `output/png/` from the current working directory.

## Notes

- These scripts are hand-maintained and aligned with `docs/architecture/*.md`.
- If architecture docs change, update both markdown and diagram scripts together.
