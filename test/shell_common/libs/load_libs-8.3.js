// All changes required to run JS tests in legacy / qa-tests directories should go in this file.
// Assume that the shell is running from either test/legacy42/ or test/qa-tests directories for load paths.

// This file intentionally loads the libs for 8.1, since the changes for 8.1 are the same as what we
// need for 8.3.

if (typeof TestData == "undefined") {
  print('Initialising TestData in load_libs.8.3.js')
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

ToolTest.prototype.runTool = function () {
    let a = ["mongo" + arguments[0]];

    let hasdbpath = false;
    let hasDialTimeout = false;

    for (let i = 1; i < arguments.length; i++) {
        a.push(arguments[i]);
        if (arguments[i] === "--dbpath") hasdbpath = true;
        if (arguments[i] === "--dialTimeout") hasDialTimeout = true;
    }

    if (!hasdbpath) {
        a.push("--host");
        a.push("127.0.0.1:" + this.port);
    }

    if (!hasDialTimeout) {
        a.push("--dialTimeout");
        a.push("30");
    }

    return runMongoProgram.apply(null, a);
};
