package main

// secrets.go — runtime.secrets binding (stub; implementation follows in subsequent commits).
// The import below anchors go-keyring in go.mod so go mod tidy does not prune it
// before the full implementation lands.

import (
	_ "github.com/zalando/go-keyring"
)
