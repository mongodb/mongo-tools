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

func EvgIsPatch() bool {
	return get("EVG_IS_PATCH") != ""
}

func EvgBuildID() (string, error) {
	return mustGet("EVG_BUILD_ID")
}

func EvgVariant() (string, error) {
	return mustGet("EVG_VARIANT")
}
