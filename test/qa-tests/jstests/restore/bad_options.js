(function() {

  load("jstests/configs/standard_dump_targets.config.js");

  // Tests running mongorestore with bad command line options.

  jsTest.log('Testing running mongorestore with bad'+
            ' command line options');

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/server.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var toolTest = new ToolTest('incompatible_flags', TOOLS_TEST_CONFIG);
  toolTest.startDB('foo');

  var sslOptions = ['--ssl', '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem', '--sslAllowInvalidHostnames'];

  // run restore with both --objcheck and --noobjcheck specified
  var ret = toolTest.runTool.apply(toolTest, ['restore',
    '--objcheck', '--noobjcheck']
    .concat(getRestoreTarget('restore/testdata/dump_empty'))
    .concat(sslOptions));
  assert.neq(0, ret);

  // run restore with --oplogLimit with a bad timestamp
  ret = toolTest.runTool.apply(toolTest, ['restore',
    '--oplogReplay', '--oplogLimit',
    'xxx']
    .concat(getRestoreTarget('restore/testdata/dump_with_oplog'))
    .concat(sslOptions));
  assert.neq(0, ret);

  // run restore with a negative --w value
  ret = toolTest.runTool.apply(toolTest, ['restore',
    '--w', '-1']
    .concat(getRestoreTarget('jstests/restore/testdata/dump_empty'))
    .concat(sslOptions));
  assert.neq(0, ret);

  // run restore with an invalid db name
  ret = toolTest.runTool.apply(toolTest, ['restore',
    '--db', 'billy.crystal']
    .concat(getRestoreTarget('jstests/restore/testdata/blankdb'))
    .concat(sslOptions));
  assert.neq(0, ret);

  // run restore with an invalid collection name
  ret = toolTest.runTool.apply(toolTest, ['restore',
    '--db', 'test',
    '--collection', '$money']
    .concat(getRestoreTarget('jstests/restore/testdata/blankcoll/blank.bson'))
    .concat(sslOptions));
  assert.neq(0, ret);

  // run restore with an invalid verbosity value
  ret = toolTest.runTool.apply(toolTest, ['restore',
    '-v', 'torvalds']
    .concat(getRestoreTarget('restore/testdata/dump_empty'))
    .concat(sslOptions));
  assert.neq(0, ret);

  // success
  toolTest.stop();

}());
