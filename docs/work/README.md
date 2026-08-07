# Active Work Coordination

`active/` is the repository's coordination ledger for meaningful active work.
Each work item has one owner and records its outcome, claimed surfaces,
dependencies, contract impact, verification, and handoff state. This directory
supplements rather than replaces the approved development plan: the plan owns
sequencing; a work item owns current implementation coordination.

Move a finished, abandoned, or superseded item to `archive/` when that state is
recorded. Keep claims narrow and do not treat them as file locks.

Each active item must include:

- Status, owner, collaborators, start time, and last update
- Development-plan and detailed-plan references
- Outcome, scope, non-goals, and claimed surfaces
- Dependencies, blockers, contract and data impact
- Current state, verification, handoff, and completion state
