// All changes required to stand up a cluster for the integration tests should go in this file.
// Load paths are written relative to a sibling of shell_common, and resolve through the baseUrl in
// jsconfig.json, which the cluster-setup scripts copy to the repo root before starting the shell.

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
