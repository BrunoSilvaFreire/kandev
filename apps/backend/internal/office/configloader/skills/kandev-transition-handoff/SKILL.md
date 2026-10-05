---
name: kandev-transition-handoff
description: Ship a linked handoff document with every workflow step transition so the receiving agent resumes without re-searching the repository or conversation.
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo, worker, specialist, assistant, reviewer]
---

# Transition handoffs

Every transition of a task between workflow steps is a handoff between agents.
The receiving agent has not seen the work: it has a different context window, a
different model, and often a different working tree. Make the handoff durable.

## Rule

Before you move a task with `move_task_kandev`, and before you call
`step_complete_kandev` from a step whose `on_turn_complete` advances the task,
write a handoff document and reference it in the move.

Do not paste the handoff into the move itself. Write it once as a task document,
then pass its document key in the transition's `entry_options.instructions` so
the next agent can read it in full.

### Exception: terminal transitions

A transition that ends the task — into a step that completes on entry, such as
"Done", or that otherwise has no receiving agent — does not need a separate
handoff document. Its own artifacts are the record: on a review pass that means
the `review` task document and the `show_walkthrough_kandev` walkthrough, plus
the `TODO:` comments the reviewer added for non-blocking suggestions. Make those
artifacts link the changed files and lines exactly as a handoff would.

## Write the handoff document

Use `write_task_document_kandev`:

```json
{
  "document_key": "handoff-review-to-implement",
  "type": "handoff",
  "title": "Handoff: Review -> Implement",
  "content": "..."
}
```

Pick a key that names the transition (`handoff-architect-to-implement`,
`handoff-implement-to-architect`, `handoff-review-to-implement`, ...). Never
overwrite a key that already holds a different investigation: call
`list_task_documents_kandev`, then use the next free key (`handoff-...-2`,
`handoff-...-3`). Earlier handoffs stay readable for the agent that follows.
Document keys are free strings: custom keys are first-class. `plan`, `spec`,
`spike`, `notes`, `review`, and `handoff` are only well-known conventions.

## Reference it in the move

```
move_task_kandev(
  task_id,
  transition="<name>",
  entry_options={"instructions":
    "<WHAT CHANGED / WHERE TO RESUME>. Full handoff: get_task_document_kandev(document_key=\"handoff-...\")."
  }
)
```

The transition's own configured `instructions` are prepended automatically; your
`entry_options.instructions` carry the document key and the one-line resume point.

## What the document must contain

1. **Objective and resume point.** One sentence on the task's goal, then the
   exact place to continue ("implement step 3 of the plan", "re-review
   `internal/foo/bar.go`").
2. **What changed.** Every file created, modified, or deleted, each with a
   durable link (see below), and one line on what changed in it.
3. **Evidence.** The verification commands you ran with their results, plus any
   test output, run ids, or logs the next agent should trust.
4. **Decisions and rationale.** What you chose and why, so it is not re-litigated.
5. **Open questions and blockers.** Anything unresolved, with the evidence.

State only facts you verified. If evidence is missing, say so; never invent it.

## Link file changes so no one has to search

The whole point is that the next agent does not have to grep the repository.
Reference every load-bearing file with a stable link, not a bare path.

- **Committed change (preferred).** Pin to the commit so the link survives later
  edits. Use line ranges when a specific region matters:
  `[internal/foo/bar.go:120-148](https://github.com/<owner>/<repo>/blob/<sha>/internal/foo/bar.go#L120-L148)`
- **Commit itself.** `[Commit <short-sha>](https://github.com/<owner>/<repo>/commit/<sha>)`
- **Pull request.** `[PR #123](https://github.com/<owner>/<repo>/pull/123)`
- **Uncommitted change (working tree).** There is no commit to pin. Link the task
  document to the diff summary and cite `path:line` from the step's step-complete
  summary; note explicitly that the change is uncommitted.
- **Existing code you relied on.** Link the same way as a committed change — a
  `<sha>` link to the file and line is better than a path the reader must resolve.
- **External reference.** Link the issue, spec, or artifact in full.

Always pair a link with a `path:line` citation so it stays useful even without
network access.

## Anti-patterns

- A transition with no handoff document.
- A handoff that says "see the conversation" or "as discussed above".
- Bare file names with no link and no line range.
- Overwriting another investigation's document key.
- Pasting the full document into `entry_options.instructions` instead of the key.
