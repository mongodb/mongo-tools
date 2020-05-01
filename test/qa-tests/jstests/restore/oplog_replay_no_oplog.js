(function() {

  load("jstests/configs/standard_dump_targets.config.js");
  // Tests using mongorestore with --oplogReplay when no oplog.bson file is present.

  jsTest.log('Testing restoration with --oplogReplay and no oplog.bson file');

  var TOOLS_TEST_CONFIG = {
    tlsMode: "requireTLS",
    tlsCertificateKeyFile: "jstests/libs/client.pem",
    tlsCAFile: "jstests/libs/ca.pem",
    tlsAllowInvalidHostnames: "",
  };
  var toolTest = new ToolTest('oplog_replay_no_oplog', TOOLS_TEST_CONFIG);
  var commonToolArgs = getCommonToolArguments();
  toolTest.startDB('foo');

  // run the restore, with a dump directory that has no oplog.bson file
  var ret = toolTest.runTool.apply(this, ['restore', '--oplogReplay']
    .concat(getRestoreTarget('restore/testdata/dump_empty'))
    .concat(commonToolArgs));
  assert.neq(0, ret);

  // success
  toolTest.stop();

}());
