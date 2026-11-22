// Command package-macos creates Finder entries while keeping the four real
// executables together, so services and application-side data keep their paths.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	branding "reia-clash/ico"
)

func main() {
	directory := flag.String("dir", ".", "directory containing the four built macOS executables")
	flag.Parse()
	icon, err := os.ReadFile("ico/reia-clash.icns")
	check(err)
	for _, part := range []struct{ program, name string }{{"reia", "ReiaClash"}, {"reia-agent", "ReiaClash Agent"}, {"reia-tray", "ReiaClash Tray"}, {"reia-service", "ReiaClash Service"}} {
		if _, err := os.Stat(filepath.Join(*directory, part.program)); err != nil {
			check(err)
		}
		contents := filepath.Join(*directory, part.name+".app", "Contents")
		check(os.MkdirAll(filepath.Join(contents, "MacOS"), 0755))
		check(os.MkdirAll(filepath.Join(contents, "Resources"), 0755))
		check(os.WriteFile(filepath.Join(contents, "Resources", "reia-clash.icns"), icon, 0644))
		wrapper := "#!/bin/sh\nset -eu\nprojectDir=$(CDPATH= cd \"$(dirname \"$0\")/../../..\" && pwd)\nexec \"$projectDir/" + part.program + "\" \"$@\"\n"
		entry := filepath.Join(contents, "MacOS", part.program)
		check(os.WriteFile(entry, []byte(wrapper), 0755))
		check(os.Chmod(entry, 0755))
		plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>%s</string>
<key>CFBundleDisplayName</key><string>%s</string>
<key>CFBundleIdentifier</key><string>jp.reia.clash.%s</string>
<key>CFBundleExecutable</key><string>%s</string>
<key>CFBundleIconFile</key><string>reia-clash.icns</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleVersion</key><string>1</string>
<key>LSUIElement</key><true/>
</dict></plist>
`, branding.AppName, branding.AppName, part.program, part.program)
		check(os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(plist), 0644))
	}
	fmt.Println("macOS application entries generated with ReiaClash icons")
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
