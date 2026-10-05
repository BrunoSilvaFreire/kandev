// Package activitycursor encodes and validates the opaque keyset cursor used
// by the task activity projection. A cursor is bound to the task that produced
// it; a mismatch or malformed value is a validation error.
package activitycursor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const cursorVersion = 1

// ErrInvalidCursor is returned for any malformed or mismatched cursor.
var ErrInvalidCursor = errors.New("invalid activity cursor")

type wireCursor struct {
	Version int    `json:"v"`
	TaskID  string `json:"k"`
	At      string `json:"t"`
}

// Encode serializes the exclusive cursor for a task.
func Encode(taskID string, before time.Time) string {
	payload, err := json.Marshal(wireCursor{
		Version: cursorVersion,
		TaskID:  taskID,
		At:      before.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

// Decode parses and validates a cursor against the task that must have
// produced it.
func Decode(raw, taskID string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("%w: empty", ErrInvalidCursor)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: not base64url", ErrInvalidCursor)
	}
	var wire wireCursor
	if err := json.Unmarshal(decoded, &wire); err != nil {
		return time.Time{}, fmt.Errorf("%w: not json", ErrInvalidCursor)
	}
	if wire.Version != cursorVersion || wire.TaskID != taskID || wire.At == "" {
		return time.Time{}, fmt.Errorf("%w: mismatch", ErrInvalidCursor)
	}
	at, err := time.Parse(time.RFC3339Nano, wire.At)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: bad timestamp", ErrInvalidCursor)
	}
	return at, nil
}
