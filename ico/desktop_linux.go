//go:build linux && !android

package branding

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InstallDesktopEntries gives desktop environments an icon for each Reia
// executable. Only the main launcher appears in application menus.
func InstallDesktopEntries() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	icon, err := DesktopIconPath()
	if err != nil {
		return err
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	for _, part := range []string{"reia", "reia-agent", "reia-tray", "reia-service"} {
		program := filepath.Join(filepath.Dir(executable), part)
		if info, err := os.Stat(program); err != nil || !info.Mode().IsRegular() {
			continue
		}
		path := filepath.Join(dataHome, "applications", part+".desktop")
		old, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && !bytes.HasPrefix(old, []byte("# Managed by ReiaClash\n")) {
			return fmt.Errorf("桌面入口 %q 不是由 ReiaClash 管理，保留原文件", path)
		}
		if err := writeAsset(path, []byte(desktopEntry(part, program, icon))); err != nil {
			return err
		}
	}
	return nil
}

func desktopEntry(part, executable, icon string) string {
	name := AppName
	hidden := ""
	if part != "reia" {
		hidden = "NoDisplay=true\n"
	}
	// Exec has both Desktop Entry string escaping and quoted-argument
	// escaping; literal percent signs must not become field codes.
	arg := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`", "%", "%%").Replace(executable)
	return "# Managed by ReiaClash\n[Desktop Entry]\nType=Application\nName=" + name + "\nExec=" + desktopString(`"`+arg+`"`) + "\nIcon=" + desktopString(icon) + "\nTerminal=false\nCategories=Network;\n" + hidden
}

func desktopString(value string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(value)
}
