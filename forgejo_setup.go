//go:build tools
// +build tools

package main

import (
	// Mock usage of forgego to enable SDK installation.
	_ "codeberg.org/mvdkleijn/forgejo-sdk/forgejo/v2"
)
