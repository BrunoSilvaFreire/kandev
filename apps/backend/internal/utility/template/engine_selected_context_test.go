package template

import "testing"

func TestResolveSelectedContext(t *testing.T) {
	got, err := NewEngine().Resolve("Prompt:\n{{SelectedContext}}", &Context{SelectedContext: "[Spec]\nKeep the retry limit."})
	if err != nil {
		t.Fatal(err)
	}
	if got != "Prompt:\n[Spec]\nKeep the retry limit." {
		t.Fatalf("Resolve() = %q", got)
	}
}
