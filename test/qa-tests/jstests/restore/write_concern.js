(function() {

  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/client.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var toolTest = new ToolTest('write_concern', TOOLS_TEST_CONFIG);
  var commonToolArgs = getCommonToolArguments();

  var rs = new ReplSetTest({
    name: "rpls",
    nodes: 3,
    useHostName: true,
    settings: {chainingAllowed: false},
    nodeOptions: TOOLS_TEST_CONFIG
  });

  rs.startSet();
  rs.initiate();
  rs.awaitReplication();
  toolTest.port = rs.getPrimary().port;
  var dbOne = rs.getPrimary().getDB("dbOne");

  // create a test collection
  var data = [];
  for (var i=0; i<=100; i++) {
    data.push({_id: i, x: i*i});
  }
  dbOne.test.insertMany(data);
  rs.awaitReplication();

  // dump the data that we'll
  var dumpTarget = 'write_concern_dump';
  resetDbpath(dumpTarget);
  var ret = toolTest.runTool.apply(toolTest, ['dump']
    .concat(getDumpTarget(dumpTarget))
    .concat(commonToolArgs));
  assert.eq(0, ret);

  function writeConcernTestFunc(exitCode, writeConcern, name) {
    jsTest.log(name);
    var ret = toolTest.runTool.apply(toolTest, ['restore']
      .concat(writeConcern)
      .concat(getRestoreTarget(dumpTarget))
      .concat(commonToolArgs));
    assert.eq(exitCode, ret, name);
  }

  function testSetup() {
    dbOne.dropDatabase();
  }

  function noConnectTest() {
    return startMongoProgramNoConnect.apply(null, ['mongorestore',
      '--writeConcern={w:3}', '--host', rs.getPrimary().host]
      .concat(getRestoreTarget(dumpTarget))
      .concat(commonToolArgs));
  }

  // drop the database so it's empty
  dbOne.dropDatabase();

  // load and run the write concern suite
  load('jstests/libs/wc_framework.js');
  runWCTest("mongorestore", rs, toolTest, writeConcernTestFunc, noConnectTest, testSetup);

  dbOne.dropDatabase();
  rs.stopSet();
  toolTest.stop();

}());
