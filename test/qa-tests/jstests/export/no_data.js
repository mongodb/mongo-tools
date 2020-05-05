(function() {

  // Tests running mongoexport with no data in the target collection.

  jsTest.log('Testing exporting no data');

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/client.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  };
  var toolTest = new ToolTest('no_data', TOOLS_TEST_CONFIG);
  toolTest.startDB('foo');

  // run mongoexport with no data, make sure it doesn't error out
  var ret = toolTest.runTool('export',
    '--db', 'test', '--collection', 'data', '--ssl',
    '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem',
    '--sslAllowInvalidHostnames');
  assert.eq(0, ret);

  // but it should fail if --assertExists specified
  ret = toolTest.runTool('export',
    '--db', 'test', '--collection', 'data', '--assertExists',
    '--ssl', '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem',
    '--sslAllowInvalidHostnames');
  assert.neq(0, ret);

  // success
  toolTest.stop();

}());
