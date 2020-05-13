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
  var dbOne = rs.nodes[0].getDB("dbOne");

  function writeConcernTestFunc(exitCode, writeConcern, name) {
    jsTest.log(name);
    ret = toolTest.runTool.apply(toolTest, ['files',
      '-vvvvv',
      '-d', 'dbOne']
      .concat(writeConcern)
      .concat(commonToolArgs)
      .concat(['put', 'jstests/files/testdata/files1.txt']));
    assert.eq(exitCode, ret, name);
  }

  function testSetup() {
    dbOne.dropDatabase();
  }

  function noConnectTest() {
    return startMongoProgramNoConnect.apply(null, ['mongofiles',
      '-d', 'dbOne',
      '--writeConcern={w:3}',
      '--host', rs.getPrimary().host]
      .concat(commonToolArgs)
      .concat(['put', 'jstests/files/testdata/files1.txt']));
  }

  // drop the database so it's empty
  dbOne.dropDatabase();

  // load and run the write concern suite
  load('jstests/libs/wc_framework.js');
  runWCTest("mongofiles", rs, toolTest, writeConcernTestFunc, noConnectTest, testSetup);

  dbOne.dropDatabase();
  rs.stopSet();
  toolTest.stop();

}());
