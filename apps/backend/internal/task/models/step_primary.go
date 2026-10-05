package models

import "maps"

// StepPrimarySessionID extracts the designated primary session ID for the given
// workflow step from task metadata, or returns empty string if not set or invalid.
func StepPrimarySessionID(metadata map[string]interface{}, stepID string) string {
	if len(metadata) == 0 || stepID == "" {
		return ""
	}
	raw, ok := metadata[MetaKeyStepPrimarySessions]
	if !ok || raw == nil {
		return ""
	}
	switch m := raw.(type) {
	case map[string]string:
		return m[stepID]
	case map[string]interface{}:
		if val, ok := m[stepID].(string); ok {
			return val
		}
	}
	return ""
}

// StepPrimarySessions extracts the map of designated primary sessions per step
// from task metadata.
func StepPrimarySessions(metadata map[string]interface{}) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata[MetaKeyStepPrimarySessions]
	if !ok || raw == nil {
		return nil
	}
	switch m := raw.(type) {
	case map[string]string:
		return maps.Clone(m)
	case map[string]interface{}:
		result := make(map[string]string, len(m))
		for k, v := range m {
			if s, ok := v.(string); ok && s != "" {
				result[k] = s
			}
		}
		return result
	}
	return nil
}
