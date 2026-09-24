package utility

import (
	"strings"
	"sync"

	"github.com/coder/acp-go-sdk"
)

// progressToolRuneCap bounds the tool title sent in a progress frame.
const progressToolRuneCap = 60

// progressEmitter dedupes progress frames and forwards only changes to its
// reporter. A nil emitter is a no-op, so callers without a reporter pay
// nothing.
type progressEmitter struct {
	mu   sync.Mutex
	fn   ProgressReporter
	last PromptProgress
	set  bool
}

func newProgressEmitter(fn ProgressReporter) *progressEmitter {
	if fn == nil {
		return nil
	}
	return &progressEmitter{fn: fn}
}

func (e *progressEmitter) emit(p PromptProgress) {
	if e == nil || e.fn == nil {
		return
	}
	e.mu.Lock()
	if e.set && e.last == p {
		e.mu.Unlock()
		return
	}
	e.last = p
	e.set = true
	e.mu.Unlock()
	e.fn(p)
}

func (e *progressEmitter) phase(phase PromptProgressPhase) {
	e.emit(PromptProgress{Phase: phase})
}

func (e *progressEmitter) tool(title string) {
	e.emit(PromptProgress{Phase: PromptPhaseTool, Tool: truncateProgressTool(title)})
}

// observe maps one ACP session notification onto a coarse phase.
func (e *progressEmitter) observe(n acp.SessionNotification) {
	if e == nil {
		return
	}
	switch {
	case n.Update.AgentThoughtChunk != nil:
		e.phase(PromptPhaseAnalyzing)
	case n.Update.AgentMessageChunk != nil:
		e.phase(PromptPhaseGenerating)
	case n.Update.ToolCall != nil:
		if terminalToolStatus(n.Update.ToolCall.Status) {
			e.phase(PromptPhaseAnalyzing)
			return
		}
		e.tool(n.Update.ToolCall.Title)
	case n.Update.ToolCallUpdate != nil:
		if n.Update.ToolCallUpdate.Status != nil && terminalToolStatus(*n.Update.ToolCallUpdate.Status) {
			e.phase(PromptPhaseAnalyzing)
			return
		}
		if n.Update.ToolCallUpdate.Title != nil && *n.Update.ToolCallUpdate.Title != "" {
			e.tool(*n.Update.ToolCallUpdate.Title)
			return
		}
		e.phase(PromptPhaseTool)
	}
}

func terminalToolStatus(status acp.ToolCallStatus) bool {
	return status == acp.ToolCallStatusCompleted || status == acp.ToolCallStatusFailed
}

func truncateProgressTool(title string) string {
	trimmed := strings.TrimSpace(title)
	runes := []rune(trimmed)
	if len(runes) <= progressToolRuneCap {
		return trimmed
	}
	return string(runes[:progressToolRuneCap])
}
