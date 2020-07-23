/* Disabled: see TOOLS-2661
(function() {
  // This test makes sure that mongodump does not do secondary reads when talking to a mongos.
  // We do this by creating a sharded topology against a replica set where all the nodes have
  // profiling enabled. After running mongodump, we can query all of the profile collections
  // to see if the queries reached any of the secondary nodes (they shouldn't!).

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/server.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }

  var NODE_COUNT = 5;
  var st = new ShardingTest({
    shards: {
      rs0: {
        nodes: NODE_COUNT,
        useHostName: true,
        settings: {chainingAllowed: false},
      }
    },
    mongos: 1,
    config: 1,
    configReplSetTestOptions: {
      settings: {chainingAllowed: false},
    },
    other: {
      configOptions: TOOLS_TEST_CONFIG,
      mongosOptions: TOOLS_TEST_CONFIG,
      shardOptions: TOOLS_TEST_CONFIG,
      nodeOptions: TOOLS_TEST_CONFIG,
    },
    rs: TOOLS_TEST_CONFIG,
  });
  var replTest = st.rs0;
  replTest.awaitReplication();
  var conn = st.s;

  var db = conn.getDB("test");
  var replDB = replTest.getPrimary().getDB("test");

  var sslOptions = ['--ssl', '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem', '--sslAllowInvalidHostnames'];

  // whether or not this is mmapv1, this will effect some results
  var isMMAPV1 = replDB.serverStatus().storageEngine.name === "mmapv1";

  db.a.insert({a: 1});
  db.a.insert({a: 2});
  db.a.insert({a: 3});
  db.a.insert({a: 4});
  db.a.insert({a: 5});
  db.a.insert({a: 5});
  assert.eq(db.a.count(), replDB.a.count(), "count should match for mongos and mongod");

  printjson(replDB.setProfilingLevel(2));
  var secondaries = [];
  // get all the secondaries and enable profiling
  for (var i = 0; i < NODE_COUNT; i++) {
    var sDB = replTest.nodes[i].getDB("test");
    if (sDB.isMaster().secondary) {
      sDB.setProfilingLevel(2);
      secondaries.push(sDB);
    }
  }
  print("done enabling profiling");
  printjson(secondaries);

  // perform 3 queries with the shell (sanity check before using mongodump)
  assert.eq(db.a.find({a: {"$lte": 3}}).toArray().length, 3);
  assert.eq(db.a.find({a: 3}).toArray().length, 1);
  assert.eq(db.a.find({a: 5}).toArray().length, 2);
  // assert that the shell queries happened only on primaries
  profQuery= {ns: "test.a", op: "query"};
  assert.eq(replDB.system.profile.find(profQuery).count(), 3,
    "three queries should have been logged");
  for (i = 0; i < secondaries.length; i++) {
    assert.eq(secondaries[i].system.profile.find(profQuery).count(), 0,
      "no queries should be against secondaries");
  }

  print("running mongodump on mongos");
  mongosAddr = st.getConnNames()[0];
  runMongoProgram.apply(this, ["mongodump",
    "--host", st.s.host, "-vvvv"]
      .concat(sslOptions));
  assert.eq(replDB.system.profile.find(profQuery).count(), 4, "queries are routed to primary");
  printjson(replDB.system.profile.find(profQuery).toArray());

  var hintCount = replDB.system.profile.find({
    ns: "test.a",
    op: "query",
    $or: [
      // 4.0
      {"command.hint._id": 1},

      // 3.6 schema
      {"command.$snapshot": true},
      {"command.snapshot": true},

      // 3.4 and previous schema
      {"query.$snapshot": true},
      {"query.snapshot": true},
      {"query.hint._id": 1},
    ]
  }).count();
  // in a mmapv1 stored database, we should snapshot or have a query hint set.
  if (isMMAPV1) {
    assert.eq(hintCount, 1);
  } else {
    assert.eq(hintCount, 0);
  }

  // make sure the secondaries saw 0 queries
  for (i = 0; i < secondaries.length; i++) {
    print("checking secondary " + i);
    assert.eq(secondaries[i].system.profile.find(profQuery).count(), 0,
      "no dump queries should be against secondaries");
  }

}());
*/
