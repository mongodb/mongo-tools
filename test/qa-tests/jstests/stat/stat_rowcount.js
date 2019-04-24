(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }
  load("jstests/libs/output.js");
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;
  var commonToolArgs = getCommonToolArguments();
  print("common tool sargs");
  printjson(commonToolArgs);

  var toolTest = getToolTest('stat_rowcount');
  var x, pid;
  clearRawMongoProgramOutput();

  x = runMongoProgram("mongostat", "--host", toolTest.m.host, "--rowcount", 7, "--noheaders");
  assert.eq.soon(7, function() {
    return allDefaultStatRows().length;
  }, "--rowcount value is respected correctly");

  startTime = new Date();
  x = runMongoProgram("mongostat", "--host", toolTest.m.host, "--rowcount", 3, "--noheaders", 3);
  endTime = new Date();
  duration = Math.floor((endTime - startTime) / 1000);
  assert.gte(duration, 9, "sleep time affects the total time to produce a number or results");

  clearRawMongoProgramOutput();

  pid = startMongoProgramNoConnect.apply(null, ["mongostat", "--port", toolTest.port].concat(commonToolArgs));
  assert.strContains.soon('sh'+pid+'|  ', rawMongoProgramOutput, "should produce some output");
  assert.eq(exitCodeFailure, stopMongoProgramByPid(pid), "stopping should cause mongostat exit with a 'failure' code");

  x = runMongoProgram.apply(null, ["mongostat", "--port", toolTest.port - 1, "--rowcount", 1].concat(commonToolArgs));
  assert.neq(exitCodeSuccess, x, "can't connect causes an error exit code");

  x = runMongoProgram.apply(null, ["mongostat", "--rowcount", "-1"].concat(commonToolArgs));
  assert.eq(exitCodeFailure, x, "mongostat --rowcount specified with bad input: negative value");

  x = runMongoProgram.apply(null, ["mongostat", "--rowcount", "foobar"].concat(commonToolArgs));
  assert.eq(exitCodeFailure, x, "mongostat --rowcount specified with bad input: non-numeric value");

  x = runMongoProgram.apply(null, ["mongostat", "--host", "badreplset/127.0.0.1:" + toolTest.port, "--rowcount", 1].concat(commonToolArgs));
  assert.eq(exitCodeFailure, x, "--host used with a replica set string for nodes not in a replica set");

  pid = startMongoProgramNoConnect.apply(null, ["mongostat", "--host", "127.0.0.1:" + toolTest.port].concat(commonToolArgs));
  assert.strContains.soon('sh'+pid+'|  ', rawMongoProgramOutput, "should produce some output");

  MongoRunner.stopMongod(toolTest.port);

  sleep(1000);
  clearRawMongoProgramOutput();
  sleep(5 * 1000);

  const rows = allDefaultStatRows();
  assert.eq(rows.length, 0, "should stop showing new stat lines, showed " + JSON.stringify(rows) + " instead.");

  assert.eq(exitCodeFailure, stopMongoProgramByPid(pid), "mongostat should return a failure code when server goes down");
}());
