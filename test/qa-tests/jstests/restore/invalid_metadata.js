(function() {

  load("jstests/configs/standard_dump_targets.config.js");

  // Tests using mongorestore to restore data from a collection whose .metadata.json
  // file contains invalid indexes.

  jsTest.log('Testing restoration from a metadata file with invalid indexes');

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/server.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var toolTest = new ToolTest('invalid_metadata', TOOLS_TEST_CONFIG);
  var commonToolArgs = getCommonToolArguments();
  toolTest.startDB('foo');

  // run restore, targeting a collection whose metadata file contains an invalid index
  var ret = toolTest.runTool.apply(toolTest, ['restore',
    '--db', 'dbOne',
    '--collection', 'invalid_metadata']
    .concat(getRestoreTarget('jstests/restore/testdata/dump_with_invalid/dbOne/invalid_metadata.bson'))
    .concat(commonToolArgs));
  assert.neq(0, ret);

  toolTest.stop();

}());
