# Project collaboration constraints

- Once an implementation plan has been approved, execute it with subagents by default: assign each plan task to a fresh implementer subagent, require an independent task review after each task, and run a final whole-branch review before completion.
- Continue through approved plan tasks without repeated approval prompts unless execution is genuinely blocked, the plan conflicts with itself, or a new decision would materially change the approved scope.
- Explicit instructions in the current user request override these defaults.
