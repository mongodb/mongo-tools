// All changes required to stand up a cluster for the integration tests should go in this file.
// Load paths are written relative to a sibling of shell_common, and resolve through the baseUrl in
// jsconfig.json, which the cluster-setup scripts copy to the repo root before starting the shell.

// This file intentionally loads the libs for 8.1, since the changes for 8.1 are the same as what we
// need for 8.2.

if (typeof TestData == "undefined") {
  print('Initialising TestData in load_libs.8.2.js')
  TestData = new Object();
}

const {ReplSetTest} = await import('../shell_common/libs/replsettest-8.1.js');
globalThis.ReplSetTest = ReplSetTest

const {ShardingTest} = await import('../shell_common/libs/shardingtest-8.1.js');
globalThis.ShardingTest = ShardingTest

// SERVER-95628 - In 8.1 shell rawMongoProgramOutput expects a regexp argument to match the program output.
// Change it here specifically when running from 8.1 shell.
var __origRawMongoProgramOutput = rawMongoProgramOutput;
rawMongoProgramOutput = function() { return __origRawMongoProgramOutput('.*') };
