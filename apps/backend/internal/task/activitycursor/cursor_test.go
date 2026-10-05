package activitycursor

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	at := time.Date(2026, 7, 1, 12, 0, 0, 123456000, time.UTC)
	encoded := Encode("task-1", at)
	if encoded == "" {
		t.Fatal("Encode returned empty")
	}
	decoded, err := Decode(encoded, "task-1")
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !decoded.Equal(at) {
		t.Fatalf("decoded = %s, want %s", decoded, at)
	}
}

func TestDecodeRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"empty":         "",
		"not base64":    "!!!",
		"not json":      base64.RawURLEncoding.EncodeToString([]byte("nope")),
		"wrong version": base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"k":"task-1","t":"2026-07-01T12:00:00Z"}`)),
		"wrong task":    base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"k":"other","t":"2026-07-01T12:00:00Z"}`)),
		"bad time":      base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"k":"task-1","t":"not-a-time"}`)),
		"missing time":  base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"k":"task-1"}`)),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(raw, "task-1"); !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("Decode(%s) err = %v, want ErrInvalidCursor", name, err)
			}
		})
	}
}
