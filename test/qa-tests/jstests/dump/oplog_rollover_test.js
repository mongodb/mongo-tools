(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/replset_single_28_tinyoplog.config.js');
  }
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;

  var targetPath = 'dump_oplog_rollover_test';
  resetDbpath(targetPath);
  var toolTest = getToolTest('oplogRolloverTest');
  var commonToolArgs = getCommonToolArguments();

  if (!toolTest.isReplicaSet) {
    print('Nothing to do for testing oplog rollover without a replica set!');
    return assert(true);
  }

  var db = toolTest.db.getSiblingDB('foo');

  var bigObj = {x: ''};
  while (bigObj.x.length < 1024 * 1024) {
    bigObj.x += 'bacon';
  }

  // get collection initialized before we start
  db.bar.insert(bigObj);

  var dumpArgs = ['mongodump',
    '-vv',
    '--oplog',
    '--failpoints', 'PauseBeforeDumping',
    '--host', toolTest.m.host]
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);

  var pid = startMongoProgramNoConnect.apply(null, dumpArgs);
  for (var i = 0; i < 1000; ++i) {
    db.bar.insert(bigObj);
  }

  assert(waitProgram(pid) !== 0,
    'mongodump --oplog should crash sensibly on oplog rollover');

  var expectedError = 'oplog overflow: mongodump was unable to capture all ' +
    'new oplog entries during execution';
  assert.strContains.soon(expectedError, rawMongoProgramOutput,
    'mongodump --oplog failure should output the correct error message');

  toolTest.stop();
}());
