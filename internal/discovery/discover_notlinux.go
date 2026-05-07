//go:build !linux

package discovery

func linuxExtraRoots(_ string) []string { return nil }
