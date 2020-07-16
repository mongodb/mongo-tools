(function() {

  load("jstests/configs/replset_28.config.js");

  var name = 'import_write_concern';
  var toolTest = new ToolTest(name, TOOLS_TEST_CONFIG);
  var dbName = "foo";
  var colName = "bar";
  var fileTarget = "wc_mongos.csv";

  var st = new ShardingTest({
    shards: {
      rs0: {
        nodes: 3,
        useHostName: true,
        settings: {chainingAllowed: false},
      },
    },
    mongos: 1,
    config: 1,
    configReplSetTestOptions: {
      settings: {chainingAllowed: false},
      nodeOptions: TOOLS_TEST_CONFIG
    },
    other: {
      configOptions: TOOLS_TEST_CONFIG,
      mongosOptions: TOOLS_TEST_CONFIG,
      shardOptions: TOOLS_TEST_CONFIG,
      nodeOptions: TOOLS_TEST_CONFIG,
    },
    rs: TOOLS_TEST_CONFIG,
  });
  var rs = st.rs0;
  rs.awaitReplication();
  toolTest.port = st.s.port;

  var commonToolArgs = getCommonToolArguments();
  var db = st.s.getDB(dbName);

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

  function startProgramNoConnect() {
    return startMongoProgramNoConnect.apply(null, ['mongoimport',
      '--writeConcern={w:3}',
      '--host', st.s.host,
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

  // setup: export the data that we'll use
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
  runWCTest("mongoimport", rs, toolTest, writeConcernTestFunc, startProgramNoConnect, testSetup);

  db.dropDatabase();
  rs.stopSet();
  toolTest.stop();
}());
