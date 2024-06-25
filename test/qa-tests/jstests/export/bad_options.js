(function() {

  // Tests running mongoexport with bad command line options.

  jsTest.log('Testing running mongoexport with bad command line options');

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/server.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var toolTest = new ToolTest('bad_options', TOOLS_TEST_CONFIG);
  toolTest.startDB('foo');

  var sslOptions = ['--ssl', '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem', '--sslAllowInvalidHostnames'];

  // run mongoexport with a missing --collection argument
  var ret = toolTest.runTool.apply(null, ['export', '--db', 'test'].concat(sslOptions));
  assert.neq(0, ret);

  // run mongoexport with bad json as the --query
  ret = toolTest.runTool.apply(null, ['export', '--db', 'test',
    '--collection', 'data', '--query', '{ hello }'].concat(sslOptions));
  assert.neq(0, ret);

  // run mongoexport with a bad argument to --skip
  ret = toolTest.runTool.apply(null, ['export',
    '--db', 'test', '--collection', 'data', '--sort', '{a: 1}',
    '--skip', 'jamesearljones'].concat(sslOptions));
  assert.neq(0, ret);

  // run mongoexport with a bad argument to --sort
  ret = toolTest.runTool.apply(null, ['export',
    '--db', 'test', '--collection', 'data', '--sort', '{ hello }']
    .concat(sslOptions));
  assert.neq(0, ret);

  // run mongoexport with a bad argument to --limit
  ret = toolTest.runTool.apply(null, ['export', '--db', 'test',
    '--collection', 'data', '--sort', '{a: 1}', '--limit', 'jamesearljones']
    .concat(sslOptions));
  assert.neq(0, ret);

  // run mongoexport with --query and --queryFile
  ret = toolTest.runTool.apply(null, ['export', '--db', 'test',
    '--collection', 'data',
    '--query', '{"a":1}',
    '--queryFile', 'jstests/export/testdata/query.json'].concat(sslOptions));
  assert.neq(0, ret);

  // run mongoexport with a --queryFile that doesn't exist
  ret = toolTest.runTool.apply(null, ['export', '--db', 'test',
    '--collection', 'data',
    '--queryFile', 'jstests/nope'].concat(sslOptions));
  assert.neq(0, ret);

  // success
  toolTest.stop();

}());
