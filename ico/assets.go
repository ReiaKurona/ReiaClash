// Package branding contains the shared ReiaClash artwork. Keep the source
// JPEG and the generated platform assets together in this directory.
package branding

import _ "embed"

//go:generate go run ../scripts/gen-icons -windows-resources

//go:embed reia-clash-64.png
var TrayPNG []byte

//go:embed reia-clash.png
var PNG []byte
