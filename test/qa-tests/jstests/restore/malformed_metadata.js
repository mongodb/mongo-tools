(function() {

  load("jstests/configs/standard_dump_targets.config.js");
  // Tests using mongorestore to restore data from a collection with
  // a malformed metadata file.

  jsTest.log('Testing restoration from a malformed metadata file');

  var TOOLS_TEST_CONFIG = {
    tlsMode: "requireTLS",
    tlsCertificateKeyFile: "jstests/libs/client.pem",
    tlsCAFile: "jstests/libs/ca.pem",
    tlsAllowInvalidHostnames: "",
  };
  var toolTest = new ToolTest('malformed_metadata', TOOLS_TEST_CONFIG);
  var commonToolArgs = getCommonToolArguments();
  toolTest.startDB('foo');

  // run restore, targeting a collection with a malformed
  // metadata.json file.
  var ret = toolTest.runTool.apply(toolTest, ['restore',
    '--db', 'dbOne',
    '--collection', 'malformed_metadata']
    .concat(getRestoreTarget('jstests/restore/testdata/dump_with_malformed/dbOne/malformed_metadata.bson'))
    .concat(commonToolArgs));
  assert.neq(0, ret);

  toolTest.stop();

}());
