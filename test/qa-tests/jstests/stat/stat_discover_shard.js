(function() {
  load("jstests/libs/output.js");

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/server.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var st = new ShardingTest({
    name: "shard1",
    shards: {
      rs0: {
        nodes: 1,
        settings: {chainingAllowed: false},
      },
      rs1: {
        nodes: 1,
        settings: {chainingAllowed: false},
      },
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
  if ("port" in st._connections[0]) {
    // MongoDB < 4.0
    shardPorts = [st._mongos[0].port, st._connections[0].port, st._connections[1].port];
  } else {
    // MongoDB >= 4.0
    shardPorts = [st._mongos[0].port, st._rs[0].nodes[0].port, st._rs[1].nodes[0].port];
  }

  clearRawMongoProgramOutput();
  pid = startMongoProgramNoConnect("mongostat",
    "--host", st._mongos[0].host,
    "--discover",
    '--ssl',
    '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem', '--sslAllowInvalidHostnames');
  assert.soon(hasOnlyPorts(shardPorts), "--discover against a mongos sees all shards");

  st.stop();
  assert.soon(hasOnlyPorts([]), "stops showing data when hosts come down");
  assert.eq(exitCodeFailure, stopMongoProgramByPid(pid), "mongostat --discover against a sharded cluster should error when the cluster goes down");
}());
