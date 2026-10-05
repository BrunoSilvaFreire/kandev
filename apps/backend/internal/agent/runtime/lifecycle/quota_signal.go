package lifecycle

import "time"

// QuotaSignalRecorder records a classified quota or rate-limit agent failure.
// It is invoked from the failure boundary after classification and must not
// block or fail the caller; implementations persist history best-effort.
type QuotaSignalRecorder interface {
	RecordLimitHit(profileID, agentID, code string, resetHint time.Time)
}

// SetQuotaSignalRecorder installs the quota signal recorder. Passing nil
// disables recording.
func (m *Manager) SetQuotaSignalRecorder(recorder QuotaSignalRecorder) {
	m.quotaRecorder = recorder
}
