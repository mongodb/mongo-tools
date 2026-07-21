// All changes required to run JS tests in legacy / qa-tests directories should go in this file.
// Assume that the shell is running from either test/legacy42/ or test/qa-tests directories for load paths.

// This file loads the 9.0 ShardingTest/ReplSetTest libs, which are derived from the 9.0 server
// jstests. The 8.x libs cannot reliably stand up a 9.0 cluster (9.0 added priority-port
// initiation, viewless-timeseries DDL, and replicated-truncate consistency logic that the older
// libs lack), so *-latest and 9.0 sharded/replset runs must use the 9.0 libs.

if (typeof TestData == "undefined") {
  print('Initialising TestData in load_libs.9.0.js')
  TestData = new Object();
}

const {ReplSetTest} = await import('../shell_common/libs/replsettest-9.0.js');
globalThis.ReplSetTest = ReplSetTest

const {ShardingTest} = await import('../shell_common/libs/shardingtest-9.0.js');
globalThis.ShardingTest = ShardingTest

// SERVER-95628 - In 8.1 shell rawMongoProgramOutput expects a regexp argument to match the program output.
// Change it here specifically when running from 8.1 shell.
var __origRawMongoProgramOutput = rawMongoProgramOutput;
rawMongoProgramOutput = function() { return __origRawMongoProgramOutput('.*') };

// This function is copied from an earlier version of the server JS tests; it was
// removed in SERVER-109431 as dead code because nothing in the server was using it.
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
