package acpcompat

import "time"

const JunieAgentID = "junie-acp"

// LateModelOptionWait is scoped to Junie, which publishes its model option in
// a config_option_update about two seconds after session/new, not in the reply.
func LateModelOptionWait(agentID string) time.Duration {
	if agentID == JunieAgentID {
		return 5 * time.Second
	}
	return 0
}
