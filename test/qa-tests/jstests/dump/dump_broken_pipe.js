(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }
  var toolTest = getToolTest('dump_broken_pipe');
  var baseArgs = getCommonToolArguments();
  baseArgs = baseArgs.concat('--port', toolTest.port);

  if (toolTest.useSSL) {
    baseArgs = baseArgs.concat([
      '--ssl',
      '--sslPEMKeyFile', 'jstests/libs/server.pem',
      '--sslCAFile', 'jstests/libs/ca.pem',
      '--sslAllowInvalidHostnames']);
  }
  if (dump_targets === 'gzip') {
    baseArgs = baseArgs.concat('--gzip');
  }
  var dumpArgs = ['mongodump', '--archive'].concat(baseArgs);
  var ddArgs = ['dd', 'count=1000000', 'bs=1024', 'of=/dev/null'];
  if (_isWindows()) {
    dumpArgs[0] += '.exe';
  }
  dumpArgs.unshift('set -o pipefail', '&&', 'PATH=.:$PATH');

  var testDb = toolTest.db;
  testDb.dropDatabase();
  for (var i = 0; i < 5000; i++) {
    testDb.foo.insert({i: i});
    testDb.bar.insert({i: i*5});
  }
  assert.eq(5000, testDb.foo.count(), 'foo should have our test documents');
  assert.eq(5000, testDb.bar.count(), 'bar should have our test documents');

  var ret = runProgram('bash', '-c', dumpArgs.concat('|', ddArgs).join(' '));
  assert.eq(0, ret, "bash execution should succeed");

  // TODO: TOOLS-1883 Following tests are flaky
  //
  // ddArgs = ['dd', 'count=100', 'bs=1', 'of=/dev/null'];
  // ret = runProgram('bash', '-c', dumpArgs.concat('|', ddArgs).join(' '));
  // assert.neq(0, ret, "bash execution should fail");
  // assert.soon(function() {
  //   return rawMongoProgramOutput().search(/broken pipe|The pipe is being closed/);
  // }, 'should print an error message');

  testDb.dropDatabase();
  toolTest.stop();
}());
