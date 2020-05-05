(function() {

  // Tests running mongoexport with bad command line options.

  jsTest.log('Testing exporting valid or invalid namespaces');

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/client.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  };
  var toolTest = new ToolTest('system_collection', TOOLS_TEST_CONFIG);
  toolTest.startDB('foo');
  var sslOptions = ['--ssl', '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem', '--sslAllowInvalidHostnames'];

  // run mongoexport with an dot in the db name
  ret = toolTest.runTool.apply(toolTest, ['export',
    '--db', 'test.bar', '--collection', 'foo'].concat(sslOptions));
  assert.neq(0, ret);

  // run mongoexport with an " in the db name
  ret = toolTest.runTool.apply(toolTest, ['export',
    '--db', 'test"bar', '--collection', 'foo'].concat(sslOptions));
  assert.neq(0, ret);

  // run mongoexport with a system collection
  ret = toolTest.runTool.apply(toolTest, ['export',
    '--db', 'test', '--collection', 'system.foobar'].concat(sslOptions));
  assert.eq(0, ret);

  // success
  toolTest.stop();

}());
