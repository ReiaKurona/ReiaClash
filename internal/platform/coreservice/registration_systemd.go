package coreservice

import "strings"

// Keep broken registrations visible so they can be repaired without starting
// a second local supervisor alongside an existing system service.
func systemdRegistration(output string, commandErr error) Registration {
	result := Registration{Backend: "systemd", Name: "reia-clash-core.service"}
	values := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		if key, value, ok := strings.Cut(line, "="); ok {
			values[key] = strings.TrimSpace(value)
		}
	}
	load := values["LoadState"]
	result.Installed = values["FragmentPath"] != "" || load == "loaded" || load == "bad-setting" || load == "error" || load == "masked"
	switch values["ActiveState"] {
	case "active", "activating", "reloading", "deactivating":
		result.Running = true
	}
	// After daemon-reload, systemd may report absence in LoadError (even
	// with exit 0), or as a D-Bus NoSuchUnit error without any properties.
	// Never let this override evidence of a registered/live unit.
	if !result.Installed && !result.Running && (load == "not-found" && values["LoadError"] == "" || systemdNoSuchUnit(output)) {
		return result
	}
	if load == "bad-setting" || load == "error" || load == "masked" {
		result.Error = "systemd LoadState=" + load
	}
	if detail := values["LoadError"]; detail != "" {
		result.Error = strings.TrimSpace(result.Error + " " + detail)
	}
	if commandErr != nil {
		result.Error = strings.TrimSpace(output) + ": " + commandErr.Error()
	}
	return result
}

func systemdNoSuchUnit(message string) bool {
	const name = "org.freedesktop.systemd1.NoSuchUnit"
	for _, field := range strings.Fields(message) {
		if strings.TrimPrefix(field, "LoadError=") == name || field == name+":" {
			return true
		}
	}
	return false
}
