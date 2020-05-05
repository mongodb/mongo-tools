(function() {

  // Tests running mongoexport with the --jsonArray output option.

  jsTest.log('Testing exporting with --jsonArray specified');

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/client.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var toolTest = new ToolTest('json_array', TOOLS_TEST_CONFIG);
  toolTest.startDB('foo');

  // the db and collection we'll use
  var testDB = toolTest.db.getSiblingDB('test');
  var testColl = testDB.data;
  var sslOptions = ['--ssl', '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem', '--sslAllowInvalidHostnames'];

  // the export target
  var exportTarget = 'json_array_export.json';
  removeFile(exportTarget);

  // insert some data
  var data = [];
  for (var i = 0; i < 20; i++) {
    data.push({_id: i});
  }
  testColl.insertMany(data);
  // sanity check the insertion worked
  assert.eq(20, testColl.count());

  // export the data
  var ret = toolTest.runTool.apply(toolTest, ['export', '--out', exportTarget,
    '--db', 'test', '--collection', 'data', '--jsonArray'].concat(sslOptions));
  assert.eq(0, ret);

  // drop the data
  testDB.dropDatabase();

  // make sure that mongoimport without --jsonArray does not work
  ret = toolTest.runTool.apply(toolTest, ['import', '--file', exportTarget,
    '--db', 'test', '--collection', 'data'].concat(sslOptions));
  assert.neq(0, ret);

  // make sure nothing was imported
  assert.eq(0, testColl.count());

  // run mongoimport again, with --jsonArray
  ret = toolTest.runTool.apply(toolTest, ['import', '--file', exportTarget,
    '--db', 'test', '--collection', 'data', '--jsonArray'].concat(sslOptions));
  assert.eq(0, ret);

  // make sure the data was imported
  assert.eq(20, testColl.count());
  for (i = 0; i < 20; i++) {
    assert.eq(1, testColl.count({_id: i}));
  }

  // success
  toolTest.stop();

}());
