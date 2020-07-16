(function() {

  load("jstests/configs/replset_28.config.js");

  var name = 'import_write_concern';
  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/server.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var toolTest = new ToolTest(name, TOOLS_TEST_CONFIG);
  var dbName = "foo";
  var colName = "bar";
  var rs = new ReplSetTest({
    name: name,
    nodes: 3,
    useHostName: true,
    settings: {chainingAllowed: false},
    nodeOptions: TOOLS_TEST_CONFIG,
  });

  var commonToolArgs = getCommonToolArguments();
  var fileTarget = "wc.csv";
  rs.startSet();
  rs.initiate();
  rs.awaitReplication();
  toolTest.port = rs.getPrimary().port;

  var db = rs.getPrimary().getDB(dbName);

  function writeConcernTestFunc(exitCode, writeConcern, name) {
    jsTest.log(name);
    var ret = toolTest.runTool.apply(toolTest, ['import',
      '--file', fileTarget,
      '-d', dbName,
      '-c', colName]
      .concat(writeConcern)
      .concat(commonToolArgs));
    assert.eq(exitCode, ret, name);
  }

  function testSetup() {
    db.dropDatabase();
  }

  function noConnectTest() {
    return startMongoProgramNoConnect.apply(null, ['mongoimport',
      '--writeConcern={w:3}',
      '--host', rs.getPrimary().host,
      '--file', fileTarget]
      .concat(commonToolArgs));
  }

  // create a test collection
  var data = [];
  for (var i=0; i<=100; i++) {
    data.push({_id: i, x: i*i});
  }
  db.getCollection(colName).insertMany(data);
  rs.awaitReplication();

  // export the data that we'll use
  var ret = toolTest.runTool.apply(toolTest, ['export',
    '--out', fileTarget,
    '-d', dbName,
    '-c', colName]
    .concat(commonToolArgs));
  assert.eq(0, ret);

  // drop the database so it's empty
  db.dropDatabase();

  // load and run the write concern suite
  load('jstests/libs/wc_framework.js');
  runWCTest("mongoimport", rs, toolTest, writeConcernTestFunc, noConnectTest, testSetup);

  db.dropDatabase();
  rs.stopSet();
  toolTest.stop();

}());
