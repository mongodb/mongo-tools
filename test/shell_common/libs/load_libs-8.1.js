// All changes required to run JS tests in legacy / qa-tests directories should go in this file.
// Assume that the shell is running from either test/legacy42/ or test/qa-tests directories for load paths.

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
