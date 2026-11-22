//go:build !windows

package network

func SaveMeowDNS(string) error    { return nil }
func RestoreMeowDNS(string) error { return nil }
func ObserveMeowDNS(string) error { return nil }
