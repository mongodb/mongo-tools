(function() {
  load("jstests/libs/output.js");

  var st = new ShardingTest({name: "shard1", shards: 2});
  if ("port" in st._connections[0]) {
    // MongoDB < 4.0
    shardPorts = [st._mongos[0].port, st._connections[0].port, st._connections[1].port];
  } else {
    // MongoDB >= 4.0
    shardPorts = [st._mongos[0].port, st._rs[0].nodes[0].port, st._rs[1].nodes[0].port];
  }

  clearRawMongoProgramOutput();
  pid = startMongoProgramNoConnect("mongostat", "--host", st._mongos[0].host, "--discover");
  assert.soon(hasOnlyPorts(shardPorts), "--discover against a mongos sees all shards");

  st.stop();
  assert.soon(hasOnlyPorts([]), "stops showing data when hosts come down");
  assert.eq(exitCodeFailure, stopMongoProgramByPid(pid), "mongostat --discover against a sharded cluster should error when the cluster goes down");
}());
