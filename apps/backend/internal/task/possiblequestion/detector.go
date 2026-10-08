package possiblequestion

import (
	"regexp"
	"strings"
)

var (
	// fencedCodeBlockRegex matches markdown fenced code blocks: ```...``` or ~~~...~~~
	fencedCodeBlockRegex = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~")

	// choicePattern matches multiple-choice cues such as "(a)... (b)..." or "(1)... (2)..."
	choicePattern = regexp.MustCompile(`(?i)(?:\([a-d1-4]\)|\[[a-d1-4]\]|\boption\s+[1-4a-d]\b)`)

	// decisionPhrases lists high-precision decision cues in trailing prose
	decisionPhrases = []string{
		"decision for you",
		"decision for the user",
		"next move is yours",
		"next step is yours",
		"per the pipeline rules, you move",
		"per the pipeline rules",
		"awaiting your input",
		"awaiting your decision",
		"awaiting input",
		"needs your input",
		"needs your decision",
		"which option do you prefer",
		"which approach do you prefer",
		"how would you like to proceed",
		"how should we proceed",
		"should i proceed",
		"shall i proceed",
		"do you want me to",
	}
)

// Detect analyzes the content of an agent message and reports whether
// trailing prose ends with a question or decision cue.
// It strips code blocks and blockquotes first to prevent false positives.
func Detect(content string) (bool, string) {
	cleaned := cleanContent(content)
	if cleaned == "" {
		return false, ""
	}

	// Extract the trailing portion (the last paragraph / last 1-3 lines)
	trailing := extractTrailingProse(cleaned)
	if trailing == "" {
		return false, ""
	}

	lower := strings.ToLower(trailing)

	// Check for explicit choice cues (e.g., "(a)... (b)..." or "Option 1... Option 2...")
	matches := choicePattern.FindAllString(lower, -1)
	if len(matches) >= 2 {
		return true, "choice_options"
	}

	// Check for explicit decision phrases in the trailing text
	for _, phrase := range decisionPhrases {
		if strings.Contains(lower, phrase) {
			return true, phrase
		}
	}

	// Check for a trailing question mark at the end of the text or last sentence
	trimmed := strings.TrimRight(trailing, " \t\r\n*\"'")
	if strings.HasSuffix(trimmed, "?") {
		lowerTrimmed := strings.ToLower(trimmed)
		if !strings.HasSuffix(lowerTrimmed, "any questions?") && !strings.HasSuffix(lowerTrimmed, "questions?") {
			return true, "trailing_question"
		}
	}

	return false, ""
}

// cleanContent removes fenced code blocks, inline code, and blockquote lines.
func cleanContent(content string) string {
	// Strip fenced code blocks
	noCode := fencedCodeBlockRegex.ReplaceAllString(content, "")

	var lines []string
	for _, line := range strings.Split(noCode, "\n") {
		trimmedLine := strings.TrimSpace(line)
		// Ignore blockquotes (> ...)
		if strings.HasPrefix(trimmedLine, ">") {
			continue
		}
		lines = append(lines, line)
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// extractTrailingProse extracts the last paragraph or last few non-empty lines.
func extractTrailingProse(text string) string {
	paragraphs := strings.Split(text, "\n\n")
	for i := len(paragraphs) - 1; i >= 0; i-- {
		p := strings.TrimSpace(paragraphs[i])
		if p != "" {
			return p
		}
	}
	return strings.TrimSpace(text)
}
