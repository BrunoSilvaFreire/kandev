package searchcursor

import (
	"encoding/base64"
	"errors"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	binding := Binding{TaskID: "task-1", ActiveSessionID: "sess-a", Query: "needle"}
	encoded := Encode(binding, Cursor{Bucket: 0, Key: "2026-01-01 00:00:00.000001", ID: "msg-1"})
	if encoded == "" {
		t.Fatal("Encode returned empty")
	}
	decoded, err := Decode(encoded, binding)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	want := Cursor{Bucket: 0, Key: "2026-01-01 00:00:00.000001", ID: "msg-1"}
	if decoded != want {
		t.Fatalf("decoded = %#v, want %#v", decoded, want)
	}
}

func TestDecodeRejectsInvalid(t *testing.T) {
	binding := Binding{TaskID: "task-1", ActiveSessionID: "sess-a", Query: "needle"}
	cases := map[string]string{
		"empty":        "",
		"not base64":   "!!!not-base64!!!",
		"not json":     base64.RawURLEncoding.EncodeToString([]byte("not json")),
		"empty object": base64.RawURLEncoding.EncodeToString([]byte("{}")),
		"bad version":  base64.RawURLEncoding.EncodeToString([]byte(`{"v":2,"b":0,"t":"k","id":"i","k":"task-1","a":"sess-a","q":"needle"}`)),
		"bad bucket":   base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"b":2,"t":"k","id":"i","k":"task-1","a":"sess-a","q":"needle"}`)),
		"missing key":  base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"b":0,"id":"i","k":"task-1","a":"sess-a","q":"needle"}`)),
		"missing id":   base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"b":0,"t":"k","k":"task-1","a":"sess-a","q":"needle"}`)),
		"wrong task":   base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"b":0,"t":"k","id":"i","k":"other","a":"sess-a","q":"needle"}`)),
		"wrong active": base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"b":0,"t":"k","id":"i","k":"task-1","a":"sess-b","q":"needle"}`)),
		"wrong query":  base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"b":0,"t":"k","id":"i","k":"task-1","a":"sess-a","q":"other"}`)),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Decode(raw, binding)
			if !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("Decode(%s) err = %v, want ErrInvalidCursor", name, err)
			}
		})
	}
}
