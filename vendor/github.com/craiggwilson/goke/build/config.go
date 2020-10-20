package build

var (
	base            = ""
	mainFile        = "build.go"
	buildOutputFile = "build.exe"
	packages        = []string{
		"./task",
		"./task/command",
		"./task/internal",
		"./pkg/git",
		"./pkg/sh",
	}
)
