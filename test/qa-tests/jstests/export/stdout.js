// Tests running mongoexport writing to stdout.
(function() {
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;

  jsTest.log('Testing exporting to stdout');

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/client.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var toolTest = new ToolTest('stdout', TOOLS_TEST_CONFIG);
  toolTest.startDB('foo');

  // the db and collection we'll use
  var testDB = toolTest.db.getSiblingDB('test');
  var testColl = testDB.data;

  // insert some data
  var data = [];
  for (var i = 0; i < 20; i++) {
    data.push({_id: i});
  }
  testColl.insertMany(data);
  // sanity check the insertion worked
  assert.eq(20, testColl.count());

  // export the data, writing to stdout
  var ret = toolTest.runTool('export', '--db', 'test',
    '--collection', 'data',
    '--ssl',
    '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem',
    '--sslAllowInvalidHostnames');
  assert.eq(0, ret);

  // wait for full output to appear
  assert.strContains.soon('exported 20 records', rawMongoProgramOutput,
    'should show number of exported records');

  // grab the raw output
  var output = rawMongoProgramOutput();

  // make sure it contains the json output
  for (i = 0; i < 20; i++) {
    assert.neq(-1, output.indexOf('{"_id":'+i+'.0}'));
  }

  // success
  toolTest.stop();
}());
