package acp

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestNormalizerAntigravityCasing(t *testing.T) {
	n := NewNormalizer("")

	t.Run("normalizeExecute with PascalCase CommandLine and Cwd", func(t *testing.T) {
		res := n.NormalizeToolCall("execute", map[string]any{
			"CommandLine": "echo hello",
			"Cwd":         "/workspace/test",
			"toolSummary": "Say hello",
			"IsDaemon":    true,
		})
		se := res.ShellExec()
		if se == nil {
			t.Fatal("expected ShellExec payload")
		}
		if se.Command != "echo hello" || se.WorkDir != "/workspace/test" || se.Description != "Say hello" || !se.Background {
			t.Fatalf("unexpected shell exec: %+v", se)
		}
	})

	t.Run("normalizeRead with AbsolutePath", func(t *testing.T) {
		res := n.NormalizeToolCall("read", map[string]any{
			"AbsolutePath": "/workspace/main.go",
		})
		rf := res.ReadFile()
		if rf == nil {
			t.Fatal("expected ReadFile payload")
		}
		if rf.FilePath != "/workspace/main.go" {
			t.Fatalf("expected /workspace/main.go, got %q", rf.FilePath)
		}
	})

	t.Run("normalizeEdit with TargetFile", func(t *testing.T) {
		res := n.NormalizeToolCall("edit", map[string]any{
			"TargetFile": "/workspace/file.go",
		})
		mf := res.ModifyFile()
		if mf == nil {
			t.Fatal("expected ModifyFile payload")
		}
		if mf.FilePath != "/workspace/file.go" {
			t.Fatalf("expected /workspace/file.go, got %q", mf.FilePath)
		}
	})

	t.Run("normalizeCodeSearch with SearchPath and Query", func(t *testing.T) {
		res := n.NormalizeToolCall("grep", map[string]any{
			"SearchPath": "/workspace/pkg",
			"Query":      "func Main",
		})
		cs := res.CodeSearch()
		if cs == nil {
			t.Fatal("expected CodeSearch payload")
		}
		if cs.Path != "/workspace/pkg" || cs.Query != "func Main" {
			t.Fatalf("unexpected code search: %+v", cs)
		}
	})

	t.Run("UpdatePayloadInput with Antigravity casing", func(t *testing.T) {
		sePayload := streams.NewShellExec("", "", "", 0, false)
		n.UpdatePayloadInput(sePayload, map[string]any{
			"CommandLine": "git status",
			"Cwd":         "/workspace",
		}, nil)
		if sePayload.ShellExec().Command != "git status" || sePayload.ShellExec().WorkDir != "/workspace" {
			t.Fatalf("unexpected updated shell exec: %+v", sePayload.ShellExec())
		}

		rfPayload := streams.NewReadFile("", 0, 0)
		n.UpdatePayloadInput(rfPayload, map[string]any{
			"AbsolutePath": "/workspace/foo.ts",
		}, nil)
		if rfPayload.ReadFile().FilePath != "/workspace/foo.ts" {
			t.Fatalf("unexpected updated read file: %+v", rfPayload.ReadFile())
		}

		mfPayload := streams.NewModifyFile("", nil)
		n.UpdatePayloadInput(mfPayload, map[string]any{
			"TargetFile": "/workspace/bar.ts",
		}, nil)
		if mfPayload.ModifyFile().FilePath != "/workspace/bar.ts" {
			t.Fatalf("unexpected updated modify file: %+v", mfPayload.ModifyFile())
		}

		csPayload := streams.NewCodeSearch("", "", "", "")
		n.UpdatePayloadInput(csPayload, map[string]any{
			"SearchPath": "/workspace/baz",
			"Query":      "findMe",
		}, nil)
		if csPayload.CodeSearch().Path != "/workspace/baz" || csPayload.CodeSearch().Query != "findMe" {
			t.Fatalf("unexpected updated code search: %+v", csPayload.CodeSearch())
		}
	})
}
