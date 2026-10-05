package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStepPrimarySessionID(t *testing.T) {
	assert.Equal(t, "", StepPrimarySessionID(nil, "step-1"))
	assert.Equal(t, "", StepPrimarySessionID(map[string]interface{}{}, "step-1"))
	assert.Equal(t, "", StepPrimarySessionID(map[string]interface{}{"other": "val"}, "step-1"))
	assert.Equal(t, "", StepPrimarySessionID(map[string]interface{}{MetaKeyStepPrimarySessions: "invalid"}, "step-1"))

	// map[string]string
	metaString := map[string]interface{}{
		MetaKeyStepPrimarySessions: map[string]string{
			"step-1": "session-1",
			"step-2": "session-2",
		},
	}
	assert.Equal(t, "session-1", StepPrimarySessionID(metaString, "step-1"))
	assert.Equal(t, "session-2", StepPrimarySessionID(metaString, "step-2"))
	assert.Equal(t, "", StepPrimarySessionID(metaString, "step-3"))

	// map[string]interface{}
	metaInterface := map[string]interface{}{
		MetaKeyStepPrimarySessions: map[string]interface{}{
			"step-1": "session-1",
			"step-2": 123, // non-string
		},
	}
	assert.Equal(t, "session-1", StepPrimarySessionID(metaInterface, "step-1"))
	assert.Equal(t, "", StepPrimarySessionID(metaInterface, "step-2"))
	assert.Equal(t, "", StepPrimarySessionID(metaInterface, "step-3"))
}

func TestStepPrimarySessions(t *testing.T) {
	assert.Nil(t, StepPrimarySessions(nil))
	assert.Nil(t, StepPrimarySessions(map[string]interface{}{}))
	assert.Nil(t, StepPrimarySessions(map[string]interface{}{MetaKeyStepPrimarySessions: 42}))

	meta := map[string]interface{}{
		MetaKeyStepPrimarySessions: map[string]interface{}{
			"step-1": "session-1",
			"step-2": "",
			"step-3": 456,
		},
	}
	res := StepPrimarySessions(meta)
	assert.Equal(t, map[string]string{"step-1": "session-1"}, res)

	metaString := map[string]interface{}{
		MetaKeyStepPrimarySessions: map[string]string{
			"step-1": "session-1",
		},
	}
	resString := StepPrimarySessions(metaString)
	assert.Equal(t, map[string]string{"step-1": "session-1"}, resString)
}
