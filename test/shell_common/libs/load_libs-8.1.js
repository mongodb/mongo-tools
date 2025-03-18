// All changes required to run JS tests in legacy / qa-tests directories should go in this file.
// Assume that the shell is running from either test/legacy42/ or test/qa-tests directories for load paths.

// Just load 8.0's replsettest, our legacy shell tests haven't changed in forever, so it shouldn't matter.
const {ReplSetTest} = await import('../shell_common/libs/replsettest-8.0.js');
globalThis.ReplSetTest = ReplSetTest

// in 8.1 shell rawMongoProgramOutput expects one parameter. Change it here specifically when running from 8.1 shell.
var __origRawMongoProgramOutput = rawMongoProgramOutput;
rawMongoProgramOutput = function() { return __origRawMongoProgramOutput('.*') };
