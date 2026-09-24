Extract a facts-only resume handoff from the conversation below, so a new agent
can continue the work without reading the original conversation.

## Conversation:
{{ConversationHistory}}

## Instructions:
1. Report only facts stated in the conversation. Do not speculate, recommend an
   approach, or rank options.
2. If the conversation does not state something, write "unknown". Never invent
   file paths, commit hashes, decisions, or next steps.
3. Name files, commits, commands, and branches exactly as they appear in the
   conversation.
4. Include the previous agent's planned next steps only if it explicitly stated
   them. If it did not, write "Previous agent did not document planned next
   steps."

## Output Format:
Return ONLY the resume prompt text with these sections, no preamble:

Objective: one or two sentences on what was being worked on.
Decisions made: bullet list of decisions stated in the conversation.
Work completed: bullet list of concrete completed work (files, commits, tests).
Current state: where things stand, including branch and last known commit.
Next steps: the previous agent's explicitly stated next steps, or "unknown".
