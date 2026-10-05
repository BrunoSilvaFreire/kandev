// Package searchcursor encodes and validates the opaque keyset cursor used by
// task-scoped message search. A cursor is bound to the exact request that
// produced it (task, active session, normalized query); a mismatch or a
// malformed value is a validation error, never a silently ignored bound.
package searchcursor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const cursorVersion = 1

// ErrInvalidCursor is returned for any malformed or request-mismatched cursor.
var ErrInvalidCursor = errors.New("invalid search cursor")

// Binding is the request identity a cursor is valid for.
type Binding struct {
	TaskID          string
	ActiveSessionID string
	Query           string
}

// Cursor is a decoded keyset position.
type Cursor struct {
	Bucket int
	Key    string
	ID     string
}

type wireCursor struct {
	Version int    `json:"v"`
	Bucket  int    `json:"b"`
	Key     string `json:"t"`
	ID      string `json:"id"`
	TaskID  string `json:"k"`
	Active  string `json:"a"`
	Query   string `json:"q"`
}

// Encode serializes a cursor for the given request binding.
func Encode(binding Binding, cursor Cursor) string {
	payload, err := json.Marshal(wireCursor{
		Version: cursorVersion,
		Bucket:  cursor.Bucket,
		Key:     cursor.Key,
		ID:      cursor.ID,
		TaskID:  binding.TaskID,
		Active:  binding.ActiveSessionID,
		Query:   binding.Query,
	})
	if err != nil {
		// A cursor built from strings cannot fail to marshal; an empty cursor
		// is the only safe degradation.
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(payload)
}

// Decode parses and validates a cursor against the request binding that must
// have produced it. A malformed value or any mismatch returns ErrInvalidCursor.
func Decode(raw string, binding Binding) (Cursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Cursor{}, fmt.Errorf("%w: empty", ErrInvalidCursor)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Cursor{}, fmt.Errorf("%w: not base64url", ErrInvalidCursor)
	}
	var wire wireCursor
	if err := json.Unmarshal(decoded, &wire); err != nil {
		return Cursor{}, fmt.Errorf("%w: not json", ErrInvalidCursor)
	}
	if wire.Version != cursorVersion {
		return Cursor{}, fmt.Errorf("%w: unsupported version", ErrInvalidCursor)
	}
	if wire.Bucket != 0 && wire.Bucket != 1 {
		return Cursor{}, fmt.Errorf("%w: invalid bucket", ErrInvalidCursor)
	}
	if wire.Key == "" || wire.ID == "" {
		return Cursor{}, fmt.Errorf("%w: missing position", ErrInvalidCursor)
	}
	if wire.TaskID != binding.TaskID ||
		wire.Active != binding.ActiveSessionID ||
		wire.Query != binding.Query {
		return Cursor{}, fmt.Errorf("%w: request mismatch", ErrInvalidCursor)
	}
	return Cursor{Bucket: wire.Bucket, Key: wire.Key, ID: wire.ID}, nil
}
