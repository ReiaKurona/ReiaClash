package agent

import (
	"reia-clash/internal/config"
	"reia-clash/internal/kernelbinary"
)

func findMihomo(settings config.Settings, managedDir string) (string, error) {
	return kernelbinary.Resolve(settings, managedDir)
}
