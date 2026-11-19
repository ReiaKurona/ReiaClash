//go:build (windows || linux || darwin) && !android

package trayui

import (
	"context"
	"testing"
)

func TestTrayExitUsesRegistrationAndExplicitChoice(t *testing.T) {
	var status trayServiceStatus
	unexpectedDialog := func(context.Context) (exitChoice, error) { t.Fatal("opened a service dialog"); return cancelExit, nil }
	if got, err := chooseExitEndpoint(t.Context(), status, unexpectedDialog); got != "/api/agent/exit" || err != nil {
		t.Fatalf("ordinary exit: endpoint=%q error=%v", got, err)
	}
	status.Service.Installed = true
	for _, test := range []struct {
		choice   exitChoice
		endpoint string
	}{
		{cancelExit, ""},
		{keepService, "/api/agent/exit"},
		{uninstallService, "/api/agent/exit/uninstall-service"},
	} {
		choose := func(context.Context) (exitChoice, error) { return test.choice, nil }
		if got, err := chooseExitEndpoint(t.Context(), status, choose); got != test.endpoint || err != nil {
			t.Fatalf("choice %v: endpoint=%q error=%v", test.choice, got, err)
		}
	}
	status.Service.Installed = false
	status.Delegated = true
	if got, err := chooseExitEndpoint(t.Context(), status, func(context.Context) (exitChoice, error) { return keepService, nil }); got != "/api/agent/exit" || err != nil {
		t.Fatalf("delegated exit: endpoint=%q error=%v", got, err)
	}
}
