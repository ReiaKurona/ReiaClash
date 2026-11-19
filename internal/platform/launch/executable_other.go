//go:build !windows

package launch

import "path/filepath"

func ResolveExecutable(path string) (string, error) { return filepath.EvalSymlinks(path) }
