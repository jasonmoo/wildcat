package golang

// This file demonstrates //go:embed directives for testing the package command.

import (
	"embed"
)

//go:embed embed.go
var _ string

//go:embed embed.go format.go
var _ embed.FS
