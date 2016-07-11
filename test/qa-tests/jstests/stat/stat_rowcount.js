(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }
  load("jstests/libs/mongostat.js");
  var commonToolArgs = getCommonToolArguments();
  print("common tool sargs");
  printjson(commonToolArgs);

  var toolTest = getToolTest('stat_rowcount');
  var x, pid;
  clearRawMongoProgramOutput();

  x = runMongoProgram("mongostat", "--host", toolTest.m.host, "--rowcount", 7, "--noheaders");
  foundRows = rawMongoProgramOutput().split("\n").filter(function(r) {
    return r.match(rowRegex);
  });
  assert.eq(foundRows.length, 7, "--rowcount value is respected correctly");

  startTime = new Date();
  x = runMongoProgram("mongostat", "--host", toolTest.m.host, "--rowcount", 3, "--noheaders", 3);
  endTime = new Date();
  duration = Math.floor((endTime - startTime) / 1000);
  assert.gte(duration, 9, "sleep time affects the total time to produce a number or results");

  clearRawMongoProgramOutput();

  pid = startMongoProgramNoConnect.apply(null, ["mongostat", "--port", toolTest.port].concat(commonToolArgs));
  sleep(1000);
  assert.eq(exitCodeStopped, stopMongoProgramByPid(pid), "stopping should cause mongostat exit with a 'stopped' code");

  x = runMongoProgram.apply(null, ["mongostat", "--port", toolTest.port - 1, "--rowcount", 1].concat(commonToolArgs));
  assert.neq(exitCodeSuccess, x, "can't connect causes an error exit code");

  x = runMongoProgram.apply(null, ["mongostat", "--rowcount", "-1"].concat(commonToolArgs));
  assert.eq(exitCodeBadOptions, x, "mongostat --rowcount specified with bad input: negative value");

  x = runMongoProgram.apply(null, ["mongostat", "--rowcount", "foobar"].concat(commonToolArgs));
  assert.eq(exitCodeBadOptions, x, "mongostat --rowcount specified with bad input: non-numeric value");

  x = runMongoProgram.apply(null, ["mongostat", "--host", "badreplset/127.0.0.1:" + toolTest.port, "--rowcount", 1].concat(commonToolArgs));
  assert.eq(exitCodeErr, x, "--host used with a replica set string for nodes not in a replica set");

  pid = startMongoProgramNoConnect.apply(null, ["mongostat", "--host", "127.0.0.1:" + toolTest.port].concat(commonToolArgs));
  sleep(2000);

  MongoRunner.stopMongod(toolTest.port);
  sleep(7000); // 1 second for the current sleep time, 5 seconds for the connection timeout, 1 second for fuzz
  assert.eq(exitCodeStopped, stopMongoProgramByPid(pid), "mongostat shouldn't error out when the server goes down");
}());
