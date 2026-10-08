package possiblequestion

import (
	"testing"
)

func TestDetect(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantMatched bool
		wantCue     string
	}{
		{
			name:        "empty content",
			content:     "",
			wantMatched: false,
		},
		{
			name:        "whitespace only",
			content:     "   \n\t  \n  ",
			wantMatched: false,
		},
		{
			name:        "normal statement completed",
			content:     "I finished implementing the user profile page and all tests pass successfully.",
			wantMatched: false,
		},
		{
			name:        "trailing question mark",
			content:     "I have prepared the changes. Can we merge this now?",
			wantMatched: true,
			wantCue:     "trailing_question",
		},
		{
			name:        "trailing question with markdown quotes/formatting",
			content:     "I've updated the schema.\n\nShould we also update the documentation?",
			wantMatched: true,
			wantCue:     "trailing_question",
		},
		{
			name:        "explicit choice options",
			content:     "I've analyzed both approaches:\n(a) migrate the database now\n(b) wait until the next release window",
			wantMatched: true,
			wantCue:     "choice_options",
		},
		{
			name:        "decision for you phrase",
			content:     "The tests passed.\n\nDecision for you: whether we want to enable caching by default.",
			wantMatched: true,
			wantCue:     "decision for you",
		},
		{
			name:        "next move is yours",
			content:     "I didn't move it. Per the pipeline rules, next move is yours.",
			wantMatched: true,
			wantCue:     "next move is yours",
		},
		{
			name:        "awaiting your input",
			content:     "Changes are ready on the branch, awaiting your input before continuing.",
			wantMatched: true,
			wantCue:     "awaiting your input",
		},
		{
			name:        "routine sign-off: let me know if you have questions",
			content:     "Everything is in place. Let me know if you have any questions.",
			wantMatched: false,
		},
		{
			name:        "routine sign-off: please let me know",
			content:     "All changes are saved. Please let me know if you'd like any adjustments.",
			wantMatched: false,
		},
		{
			name:        "routine sign-off: any questions",
			content:     "Tests pass cleanly.\n\nAny questions?",
			wantMatched: false,
		},
		{
			name:        "question inside code block ignored",
			content:     "Here is the code:\n```go\n// Should we cache this result?\nfunc cache() {}\n```\nAll done.",
			wantMatched: false,
		},
		{
			name:        "question inside blockquote ignored",
			content:     "> What if the user cancels the subscription?\n\nWe handle this by revoking tokens immediately. Everything has been deployed.",
			wantMatched: false,
		},
		{
			name:        "question early in body but trailing paragraph is declarative statement",
			content:     "Why did the test fail earlier? It was an expired mock token.\n\nI updated the token expiration in the fixture and verified all 12 tests pass cleanly.",
			wantMatched: false,
		},
		{
			name:        "spike-2 case: 46bc3fa2 trailing Next move is yours",
			content:     "I have reviewed all the diffs and verified the criteria.\n\nNext move is yours?",
			wantMatched: true,
			wantCue:     "next move is yours",
		},
		{
			name:        "spike-2 case: 545d4c71 options choice",
			content:     "I am not calling step_complete.\n\nWe can either (a) create a migration or (b) run a backfill script. Which approach do you prefer?",
			wantMatched: true,
			wantCue:     "choice_options",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, cue := Detect(tt.content)
			if matched != tt.wantMatched {
				t.Errorf("Detect() matched = %v, want %v (cue: %q)", matched, tt.wantMatched, cue)
			}
			if tt.wantMatched && tt.wantCue != "" && cue != tt.wantCue {
				t.Errorf("Detect() cue = %q, want %q", cue, tt.wantCue)
			}
		})
	}
}

func TestSpike2FixtureCorpus(t *testing.T) {
	// 5 high-confidence hits from spike-2 Q1 measurement
	highConfidenceCases := []struct {
		taskID  string
		content string
	}{
		{
			taskID:  "2b8bf8b5",
			content: "I didn't move it. Per the pipeline rules, you move it to Done.",
		},
		{
			taskID:  "46bc3fa2",
			content: "Next move is yours?",
		},
		{
			taskID:  "5ad0573e",
			content: "Nothing is committed, and I did not touch your main. Needs your decision on how to proceed.",
		},
		{
			taskID:  "545d4c71-a",
			content: "I am not calling step_complete. Want me to (a) investigate and fix that app-wide boot loop, or (b) call step_complete_kandev now?",
		},
		{
			taskID:  "545d4c71-b",
			content: "Choice: (a) migrate now or (b) postpone? Which approach do you prefer?",
		},
	}

	for _, tc := range highConfidenceCases {
		t.Run("high_confidence_"+tc.taskID, func(t *testing.T) {
			matched, cue := Detect(tc.content)
			if !matched {
				t.Errorf("Spike-2 high confidence case %s failed to detect: cue=%q", tc.taskID, cue)
			}
		})
	}

	// Routine sign-offs that should NOT match
	routineSignOffs := []string{
		"Everything looks good. Let me know if you have any questions.",
		"I've pushed the fix. Please let me know if anything looks off.",
		"Verification complete.\n\nAny questions?",
		"Done with implementation.",
	}
	for i, text := range routineSignOffs {
		matched, cue := Detect(text)
		if matched {
			t.Errorf("Routine sign-off %d unexpectedly matched with cue %q", i, cue)
		}
	}
}
