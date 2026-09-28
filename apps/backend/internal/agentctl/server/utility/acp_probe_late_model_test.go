package utility

import (
	"context"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

func lateModelOptions() []acp.SessionConfigOption {
	category := acp.SessionConfigOptionCategoryModel
	return []acp.SessionConfigOption{{Select: &acp.SessionConfigOptionSelect{
		Id: "model", Category: &category, Type: "select",
	}}}
}

func configUpdate(options []acp.SessionConfigOption) acp.SessionNotification {
	return acp.SessionNotification{Update: acp.SessionUpdate{
		ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{ConfigOptions: options},
	}}
}

func TestACPProbeNotificationStateWaitForModelOption_ReceivesLaterModel(t *testing.T) {
	state := newACPProbeNotificationState("junie-acp")
	result := make(chan []acp.SessionConfigOption, 1)
	errs := make(chan error, 1)
	go func() {
		options, ok, err := state.waitForModelOption(context.Background(), time.Second)
		if !ok {
			errs <- err
			return
		}
		result <- options
	}()

	state.handle(configUpdate(nil))
	state.handle(configUpdate(lateModelOptions()))

	select {
	case got := <-result:
		if findSelectConfigOption(got, acp.SessionConfigOptionCategoryModel) == nil {
			t.Fatal("model option missing from returned update")
		}
	case err := <-errs:
		t.Fatalf("waitForModelOption() = no update, %v; want model update", err)
	case <-time.After(time.Second):
		t.Fatal("waitForModelOption did not return")
	}
}

func TestACPProbeNotificationStateWaitForModelOption_UsesExistingModel(t *testing.T) {
	state := newACPProbeNotificationState("junie-acp")
	state.handle(configUpdate(lateModelOptions()))

	got, ok, err := state.waitForModelOption(context.Background(), time.Second)
	if err != nil || !ok {
		t.Fatalf("waitForModelOption() = %#v, %t, %v; want existing model update", got, ok, err)
	}
	if findSelectConfigOption(got, acp.SessionConfigOptionCategoryModel) == nil {
		t.Fatal("model option missing from returned update")
	}
}

func TestACPProbeNotificationStateWaitForModelOption_TimesOut(t *testing.T) {
	state := newACPProbeNotificationState("junie-acp")
	got, ok, err := state.waitForModelOption(context.Background(), 50*time.Millisecond)
	if err != nil || ok || got != nil {
		t.Fatalf("waitForModelOption() = %#v, %t, %v; want nil, false, nil", got, ok, err)
	}
}

func TestACPProbeNotificationStateWaitForModelOption_RespectsCancellation(t *testing.T) {
	state := newACPProbeNotificationState("junie-acp")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, ok, err := state.waitForModelOption(ctx, time.Second)
	if err != context.Canceled || ok || got != nil {
		t.Fatalf("waitForModelOption() = %#v, %t, %v; want nil, false, context.Canceled", got, ok, err)
	}
}

// TODO: add skip-path cases (non-Junie agent ID, requested model/mode, model already in reply) asserting no wait and unchanged ConfigOptions, so the guard cannot regress into slowing other probes.
func TestApplyLateModelOption_UsesJunieModelNotification(t *testing.T) {
	state := newACPProbeNotificationState("junie-acp")
	state.handle(configUpdate(lateModelOptions()))
	response := acp.NewSessionResponse{SessionId: "session"}

	if err := applyLateModelOption(context.Background(), "junie-acp", "", "", &response, state); err != nil {
		t.Fatalf("applyLateModelOption() error = %v", err)
	}
	if findSelectConfigOption(response.ConfigOptions, acp.SessionConfigOptionCategoryModel) == nil {
		t.Fatal("applyLateModelOption() did not apply the Junie model option")
	}
}
