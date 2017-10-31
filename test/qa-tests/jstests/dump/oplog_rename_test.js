(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/replset_28.config.js');
  }
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;

  var targetPath = 'dump_oplog_uuid_test';
  resetDbpath(targetPath);
  var toolTest = getToolTest('oplogUUIDTest');
  var commonToolArgs = getCommonToolArguments();

  if (!toolTest.isReplicaSet) {
    print('Nothing to do for testing oplog rollover without a replica set!');
    return assert(true);
  }

  var db = toolTest.db.getSiblingDB('foo');

  var bigObj = {x: ''};
  while (bigObj.x.length < 1024) {
    bigObj.x += 'bacon';
  }

  var dumpArgs = ['mongodump',
    '--oplog',
    '--host', toolTest.m.host]
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);

  for (var i = 0; i < 200000; ++i) {
    db.bar0.insert(bigObj);
  }

  var pid = startMongoProgramNoConnect.apply(null, dumpArgs);

  for (var j = 0; i < 200; ++i) {
    db.getCollection("bar"+j).insert(bigObj);
    db.getCollection("bar"+j).renameCollection("bar"+(j+1));
  }

  assert(waitProgram(pid) !== 0,
    'mongodump --oplog during renames should fail');

  toolTest.stop();
}());
