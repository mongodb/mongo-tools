//go:build linter
// +build linter

package main

import _ "github.com/3rf/mongo-lint/golint"

// `+build linter` is a build constraint to avoid compiling the linter with the tools.

// `import _` allows us to track a specific version of the lint tool via go.mod and
// prevents it from being removed from the vendor directory by `go mod tidy`.

// For more information, see:
// https://github.com/golang/go/wiki/Modules#how-can-i-track-tool-dependencies-for-a-module
