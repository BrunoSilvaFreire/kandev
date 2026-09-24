package client

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInferenceStreamReadsProgressAndResult(t *testing.T) {
	body := `{"progress":{"phase":"starting"}}
{"progress":{"phase":"tool","tool":"Read"}}
{"result":{"success":true,"response":"hello","model":"m"}}
`
	var frames []utility.PromptProgress
	result, err := parseInferenceStream(strings.NewReader(body), func(p utility.PromptProgress) {
		frames = append(frames, p)
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "hello", result.Response)
	require.Len(t, frames, 2)
	assert.Equal(t, utility.PromptPhaseStarting, frames[0].Phase)
	assert.Equal(t, utility.PromptPhaseTool, frames[1].Phase)
}

func TestParseInferenceStreamFallsBackToLegacyJSON(t *testing.T) {
	legacy := `{"success":true,"response":"legacy","model":"m2"}`
	result, err := parseInferenceStream(strings.NewReader(legacy), func(utility.PromptProgress) {})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Equal(t, "legacy", result.Response)
}

func TestParseInferenceStreamLegacyPrettyJSON(t *testing.T) {
	legacy := "{\n  \"success\": true,\n  \"response\": \"pretty\"\n}"
	result, err := parseInferenceStream(strings.NewReader(legacy), func(utility.PromptProgress) {})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "pretty", result.Response)
}

func TestParseInferenceStreamMissingResultIsError(t *testing.T) {
	body := `{"progress":{"phase":"analyzing"}}`
	_, err := parseInferenceStream(strings.NewReader(body), func(utility.PromptProgress) {})
	require.Error(t, err)
}
