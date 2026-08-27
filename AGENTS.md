# AGENTS

This repository is a Go-based database project. Keep changes minimal, focused, and consistent with the existing structure.

## Working rules
- Make the requested code and configuration changes directly in the workspace when they are appropriate and clearly scoped to the task.
- Group related changes together so code, tests, and documentation remain cohesive.
- Prefer simple, well-documented solutions over broad refactors.
- Preserve the current repository layout unless a task explicitly requires a new structure.
- Use idiomatic Go style and run the relevant validation command for any changed Go code.
- In the final response, include a conventional commit message suggestion for the grouped change set.
- Use the conventional commit format: type(scope): short summary.
- Examples: feat(db): add B+Tree page cache, fix(ci): tighten GitHub Actions permissions, docs(security): add vulnerability reporting policy.
- Do not run git commit or git push unless the user explicitly asks for it.
- If behavior is ambiguous, state the assumption clearly before changing code.

## Validation
- Prefer the smallest relevant verification step, such as running focused tests or a build check for the affected area.
- If no tests exist for the affected behavior, validate with the lightest available command that checks correctness.

## Communication
- Summarize what changed and any verification performed.
- Note blockers or assumptions clearly when they affect implementation choices.