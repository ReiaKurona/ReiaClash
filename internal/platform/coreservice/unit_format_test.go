package coreservice

import (
	"errors"
	"strings"
	"testing"
)

func TestUnitPathFormats(t *testing.T) {
	for _, path := range []string{"/home/user/.config/ReiaClash", `/home/用户/a b/$HOME/100%/"quoted"\dir`} {
		unit := renderUnit(path+"/reia-service", path, 1000, 1001)
		working := strings.Split(strings.Split(unit, "WorkingDirectory=")[1], "\n")[0]
		// Mirrors systemd's working-directory parser: only % specifiers are
		// expanded, with no shell/argument unquoting. A leading quote is fatal.
		if !strings.HasPrefix(working, "/") || strings.ReplaceAll(working, "%%", "%") != path {
			t.Fatalf("WorkingDirectory changed the literal absolute path: %q", working)
		}
		if !strings.Contains(unit, "ExecStart="+systemdQuote(path+"/reia-service")+" --core-service --data-dir "+systemdQuote(path)+"\n") {
			t.Fatal("ExecStart lost argument boundaries")
		}
		for _, required := range []string{"User=1000\n", "Group=1001\n", "KillMode=control-group\n", "AmbientCapabilities=CAP_NET_ADMIN\n", "NoNewPrivileges=true\n"} {
			if !strings.Contains(unit, required) {
				t.Fatal(required)
			}
		}
	}
	if got := systemdQuote(`/a b/$VAR/%u/"x"\y`); got != `"/a b/$$VAR/%%u/\"x\"\\y"` {
		t.Fatal(got)
	}
}

func TestSystemdRegistration(t *testing.T) {
	for _, state := range []string{"loaded", "bad-setting", "error", "masked"} {
		result := systemdRegistration("LoadState="+state+"\nActiveState=inactive\nFragmentPath=/etc/systemd/system/reia-clash-core.service\n", nil)
		if !result.Installed || result.Running || (state != "loaded" && result.Error == "") {
			t.Fatal(result)
		}
	}
	for _, active := range []string{"active", "activating", "reloading", "deactivating"} {
		if result := systemdRegistration("LoadState=loaded\nActiveState="+active+"\n", nil); !result.Running {
			t.Fatal(result)
		}
	}
	result := systemdRegistration("LoadState=bad-setting\nActiveState=inactive\nLoadError=Invalid WorkingDirectory\n", nil)
	if !strings.Contains(result.Error, "Invalid WorkingDirectory") {
		t.Fatal(result)
	}
	if result := systemdRegistration("LoadState=not-found\nActiveState=inactive\n", errors.New("exit 1")); result.Installed || result.Error != "" {
		t.Fatal(result)
	}
}

func TestSystemdRemovedUnitIsNotAnUninstallFailure(t *testing.T) {
	missing := `org.freedesktop.systemd1.NoSuchUnit "Unit reia-clash-core.service not found."`
	for _, output := range []string{
		"LoadState=not-found\nActiveState=inactive\nLoadError=" + missing + "\n",
		missing,
	} {
		for _, commandErr := range []error{nil, errors.New("exit status 1")} {
			if got := systemdRegistration(output, commandErr); got.Installed || got.Running || got.Error != "" {
				t.Fatalf("removed unit: %+v", got)
			}
		}
	}
	for _, output := range []string{
		"org.freedesktop.DBus.Error.AccessDenied permission denied",
		"Failed to connect to bus: No such file or directory",
		"org.freedesktop.systemd1.NoSuchUnitFake",
		"LoadState=loaded\nActiveState=active\nLoadError=" + missing,
		"LoadState=not-found\nActiveState=deactivating\nLoadError=" + missing,
		"LoadState=not-found\nLoadError=org.freedesktop.DBus.Error.AccessDenied",
	} {
		if got := systemdRegistration(output, errors.New("exit status 1")); got.Error == "" {
			t.Fatalf("must preserve real failures/live states: %q %+v", output, got)
		}
	}
}

func TestSystemdRepairRequiresStoppedService(t *testing.T) {
	for _, state := range []string{"inactive", "failed", "active", "activating", "deactivating", "unknown", ""} {
		t.Run(state, func(t *testing.T) {
			var calls []string
			err := disableAndStopSystemdUnit("test.service", func(args ...string) error {
				calls = append(calls, strings.Join(args, " "))
				if args[0] == "stop" {
					return errors.New("bad unit file setting")
				}
				return nil
			}, func(args ...string) ([]byte, error) {
				calls = append(calls, strings.Join(args, " "))
				return []byte(state + "\n"), nil
			})
			if (err == nil) != (state == "inactive" || state == "failed") {
				t.Fatal(state, err)
			}
			if strings.Join(calls, ";") != "disable test.service;stop test.service;show test.service --property=ActiveState --value" {
				t.Fatal(calls)
			}
		})
	}
	err := disableAndStopSystemdUnit("test.service", func(...string) error { return errors.New("stop refused") }, func(...string) ([]byte, error) {
		t.Fatal("must not continue after disable failed")
		return nil, nil
	})
	if err == nil {
		t.Fatal("disable failure ignored")
	}
	err = disableAndStopSystemdUnit("test.service", func(args ...string) error {
		if args[0] == "stop" {
			return errors.New("stop refused")
		}
		return nil
	}, func(...string) ([]byte, error) { return []byte("inactive"), errors.New("query failed") })
	if err == nil {
		t.Fatal("unconfirmed state ignored")
	}
}
