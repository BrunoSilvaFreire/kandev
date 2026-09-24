package utility

import (
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func collectReporter() (*[]PromptProgress, ProgressReporter) {
	frames := &[]PromptProgress{}
	return frames, func(p PromptProgress) { *frames = append(*frames, p) }
}

func TestProgressEmitterDedupesAndMapsPhases(t *testing.T) {
	frames, reporter := collectReporter()
	e := newProgressEmitter(reporter)

	e.phase(PromptPhaseAnalyzing)
	e.phase(PromptPhaseAnalyzing) // duplicate dropped
	e.phase(PromptPhaseGenerating)
	e.phase(PromptPhaseGenerating) // duplicate dropped
	e.phase(PromptPhaseCompleted)

	require.Equal(t, []PromptProgress{
		{Phase: PromptPhaseAnalyzing},
		{Phase: PromptPhaseGenerating},
		{Phase: PromptPhaseCompleted},
	}, *frames)
}

func TestProgressEmitterObserveMapsNotifications(t *testing.T) {
	frames, reporter := collectReporter()
	e := newProgressEmitter(reporter)

	e.observe(acp.SessionNotification{Update: acp.SessionUpdate{
		AgentThoughtChunk: &acp.SessionUpdateAgentThoughtChunk{Content: acp.TextBlock("hi")},
	}})
	e.observe(acp.SessionNotification{Update: acp.SessionUpdate{
		AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{Content: acp.TextBlock("hi")},
	}})
	e.observe(acp.SessionNotification{Update: acp.SessionUpdate{
		ToolCall: &acp.SessionUpdateToolCall{Title: "Read file", Status: acp.ToolCallStatusPending},
	}})
	completed := acp.ToolCallStatusCompleted
	e.observe(acp.SessionNotification{Update: acp.SessionUpdate{
		ToolCallUpdate: &acp.SessionToolCallUpdate{Status: &completed},
	}})

	require.Len(t, *frames, 4)
	assert.Equal(t, PromptProgress{Phase: PromptPhaseAnalyzing}, (*frames)[0])
	assert.Equal(t, PromptProgress{Phase: PromptPhaseGenerating}, (*frames)[1])
	assert.Equal(t, PromptProgress{Phase: PromptPhaseTool, Tool: "Read file"}, (*frames)[2])
	assert.Equal(t, PromptProgress{Phase: PromptPhaseAnalyzing}, (*frames)[3])
}

func TestProgressEmitterToolTitleTruncated(t *testing.T) {
	frames, reporter := collectReporter()
	e := newProgressEmitter(reporter)
	long := ""
	for i := 0; i < 100; i++ {
		long += "x"
	}
	e.tool(long)
	require.Len(t, *frames, 1)
	assert.Len(t, []rune((*frames)[0].Tool), progressToolRuneCap)
}

func TestNilProgressEmitterIsNoOp(t *testing.T) {
	var e *progressEmitter
	e.phase(PromptPhaseAnalyzing)
	e.tool("x")
	e.observe(acp.SessionNotification{})
}
