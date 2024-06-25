(function() {

  load("jstests/configs/standard_dump_targets.config.js");
  // Tests that running mongorestore with --objcheck on valid bson
  // files restores the data successfully.

  jsTest.log('Testing restoration with --objcheck');


  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/server.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var toolTest = new ToolTest('objcheck_valid_bson', TOOLS_TEST_CONFIG);
  var commonToolArgs = getCommonToolArguments();
  toolTest.startDB('foo');

  // where we'll put the dump
  var dumpTarget = 'objcheck_valid_bson_dump';
  resetDbpath(dumpTarget);

  // the db and collection we will use
  var testDB = toolTest.db.getSiblingDB('test');
  var testColl = testDB.coll;

  // insert some data
  var data = [];
  for (var i = 0; i < 50; i++) {
    data.push({_id: i});
  }
  testColl.insertMany(data);
  // sanity check the insert worked
  assert.eq(50, testColl.count());

  // dump the data
  var ret = toolTest.runTool.apply(toolTest, ['dump']
    .concat(getDumpTarget(dumpTarget))
    .concat(commonToolArgs));
  assert.eq(0, ret);

  // drop the data
  testDB.dropDatabase();

  // restore the data, with --objcheck
  ret = toolTest.runTool.apply(toolTest, ['restore']
    .concat(getRestoreTarget(dumpTarget))
    .concat(commonToolArgs));
  assert.eq(0, ret);

  // make sure the restore completed successfully
  assert.eq(50, testColl.count());

  // success
  toolTest.stop();

}());
