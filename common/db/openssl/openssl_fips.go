// +build ssl
// +build !darwin

package openssl

import (
	"fmt"

	"github.com/10gen/openssl"
	"github.com/mongodb/mongo-tools/common/options"
)

func init() { sslInitializationFunctions = append(sslInitializationFunctions, SetUpFIPSMode) }

func SetUpFIPSMode(opts options.ToolOptions) error {
	if err := openssl.FIPSModeSet(opts.SSLFipsMode); err != nil {
		return fmt.Errorf("couldn't set FIPS mode to %v: %v", opts.SSLFipsMode, err)
	}
	return nil
}
