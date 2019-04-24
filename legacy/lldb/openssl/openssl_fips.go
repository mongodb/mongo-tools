// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// +build ssl,!openssl_pre_1.0

package openssl

import (
	"fmt"

	"github.com/10gen/openssl"
	"github.com/mongodb/mongo-tools/legacy/options"
)

func init() {
	if openssl.FIPSModeDefined() {
		sslInitializationFunctions = append(sslInitializationFunctions, SetUpFIPSMode)
	} else {
		sslInitializationFunctions = append(sslInitializationFunctions, NoFIPSModeAvailable)
	}
}

func SetUpFIPSMode(opts options.ToolOptions) error {
	if err := openssl.FIPSModeSet(opts.SSLFipsMode); err != nil {
		return fmt.Errorf("couldn't set FIPS mode to %v: %v", opts.SSLFipsMode, err)
	}
	return nil
}

func NoFIPSModeAvailable(opts options.ToolOptions) error {
	if opts.SSLFipsMode {
		return fmt.Errorf("FIPS mode not supported")
	}
	return nil
}
