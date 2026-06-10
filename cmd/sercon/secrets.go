package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// secrets.go backs runtime.secrets: read/write string credentials in the OS
// keystore (macOS Keychain, Linux Secret Service, Windows Credential Manager)
// via github.com/zalando/go-keyring — pure-Go, no cgo. Every operation is
// confined to a sercon-owned prefix namespace: the keystore service used is
// PREFIX + name, and the script cannot influence PREFIX, so it can neither
// read nor clobber any secret outside the namespace.

// secretsPrefixOverride is set from the --secrets-prefix flag in main.go after
// flag parsing ("" = flag not given). Package-level to mirror the serve-mode
// override pattern (servePortOverride).
var secretsPrefixOverride string

// resolveSecretsPrefix picks the namespace prefix: --secrets-prefix flag, else
// the SERCON_SECRETS_PREFIX env var, else the default "sercon/".
func resolveSecretsPrefix() string {
	if secretsPrefixOverride != "" {
		return secretsPrefixOverride
	}
	if v := os.Getenv("SERCON_SECRETS_PREFIX"); v != "" {
		return v
	}
	return "sercon/"
}

// secretsAvailable is a cheap, side-effect-free advisory hint: does a keystore
// backend plausibly exist on this host? It does NOT touch the keystore (that
// would add a subprocess / D-Bus round-trip — and a possible macOS prompt — to
// every run). The authoritative answer is whether get/set/delete throw.
func secretsAvailable() bool {
	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	case "linux":
		return linuxSecretsAvailable(os.Getenv("DBUS_SESSION_BUS_ADDRESS"), os.Getenv("XDG_RUNTIME_DIR"))
	default:
		return false
	}
}

// linuxSecretsAvailable reports whether a D-Bus session — the transport for the
// Secret Service — is plausibly reachable, from the session-bus address or the
// default session-bus socket under XDG_RUNTIME_DIR. Pure (no globals) so it is
// unit-testable off Linux.
func linuxSecretsAvailable(dbusAddr, runtimeDir string) bool {
	if dbusAddr != "" {
		return true
	}
	if runtimeDir != "" {
		if _, err := os.Stat(filepath.Join(runtimeDir, "bus")); err == nil {
			return true
		}
	}
	return false
}
