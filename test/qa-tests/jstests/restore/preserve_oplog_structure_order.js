(function() {

  load('jstests/common/check_version.js');
  load("jstests/configs/standard_dump_targets.config.js");

  jsTest.log('Testing that the order of fields is preserved in the oplog');

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/server.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var toolTest = new ToolTest('ordered_oplog', TOOLS_TEST_CONFIG);
  toolTest.startDB('foo');

  var testDb = toolTest.db.getSiblingDB('test');
  testDb.createCollection("foobar");

  // this test is directly trying to apply the oplog. The document with _id that it's applying does not exist in the
  // collection. After SERVER-88158, applyOps does not upsert by default, and therefore we need to insert the document
  // into the collection first.
  if (isAtLeastVersion(testDb.version(), "8.1.0")) {
    testDb.foobar.insert({
      "_id": {
        "a": 1.0,
        "b": 2.0,
        "c": 3.0,
        "d": 5.0,
        "e": 6.0,
        "f": 7.0,
        "g": 8.0
      }
    });
  }

  // run restore, with an "update" oplog with a _id field that is a subdocument with several fields
  // { "h":{"$numberLong":"7987029173745013482"},"ns":"test.foobar",
  //   "o":{"_id":{"a":1,"b":2,"c":3,"d":5,"e":6,"f":7,"g":8},"foo":"bar"},
  //   "o2":{"_id":{"a":1,"b":2,"c":3,"d":5,"e":6,"f":7,"g":8}},"op":"u","ts":{"$timestamp":{"t":1439225650,"i":1}},"v":NumberInt(2)
  // }
  // if the _id from the "o" and the _id from the "o2" don't match then mongod complains
  // run it several times, because with just one execution there is a chance that restore randomly selects the correct order
  // With several executions the chances of all false positives diminishes.
  for (var i=0; i<10; i++) {
    var ret = toolTest.runTool('restore', '--oplogReplay',
      'jstests/restore/testdata/dump_with_complex_id_oplog',
      '--ssl',
      '--sslPEMKeyFile=jstests/libs/client.pem',
      '--sslCAFile=jstests/libs/ca.pem',
      '--sslAllowInvalidHostnames');
    assert.eq(0, ret);
  }

  toolTest.stop();

}());
