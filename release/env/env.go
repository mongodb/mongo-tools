package env

import (
	"fmt"
	"os"
)

func notFoundErr(varname string) error {
	return fmt.Errorf("env variable %q not set", varname)
}

func get(varname string) string {
	return os.Getenv(varname)
}

func mustGet(varname string) (string, error) {
	val := get(varname)
	if val == "" {
		return "", notFoundErr(varname)
	}
	return val, nil
}

// EvgIsPatch returns whether the current evergreen task is a patch,
// based on the value of an env variable set by the evg project file.
func EvgIsPatch() bool {
	return get("EVG_IS_PATCH") != ""
}

// EvgBuildID returns the build_id of the current evergreen task,
// based on the value of an env variable set by the evg project file.
func EvgBuildID() (string, error) {
	return mustGet("EVG_BUILD_ID")
}

// EvgVariant returns the variant name for the current evergreen task,
// based on the value of an env variable set by the evg project file.
func EvgVariant() (string, error) {
	return mustGet("EVG_VARIANT")
}
