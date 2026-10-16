package buildscript

// Test build tags. Each selects a set of tests that need a particular environment, and the test:*
// tasks pass the relevant one to `go test -tags`. Unit tests have the "unit" tag, so they do not
// compile when an integration run selects a different one.
//
// The same names appear literally in //go:build directives, which cannot reference constants, so
// keep these in sync with the files under test.

type TestTag string

const (
	TagUnit        TestTag = "unit"
	TagIntegration TestTag = "integration"
	TagSharded     TestTag = "sharded"
	TagKerberos    TestTag = "kerberos"
	TagAWSAuth     TestTag = "awsauth"
)

func (tt TestTag) String() string {
	return string(tt)
}
