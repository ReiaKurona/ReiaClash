package coreservice

import (
	"errors"
	"strings"
)

func disableAndStopSystemdUnit(name string, run func(...string) error, query func(...string) ([]byte, error)) error {
	if err := run("disable", name); err != nil {
		return err
	}
	if err := run("stop", name); err != nil {
		// A bad-setting unit may reject stop. Replace it only after an
		// independent query confirms there is no live service.
		output, queryErr := query("show", name, "--property=ActiveState", "--value")
		state := strings.TrimSpace(string(output))
		if queryErr != nil || state != "inactive" && state != "failed" {
			return errors.Join(err, queryErr)
		}
	}
	return nil
}
