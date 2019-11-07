(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }

  var targetPath = 'dump_query_flag_test';
  resetDbpath(targetPath);
  var toolTest = getToolTest('queryFlagTest');
  var commonToolArgs = getCommonToolArguments();
  var testDB = toolTest.db.getSiblingDB('foo');

  testDB.dropDatabase();
  assert.eq(0, testDB.bar.count());
  testDB.getSiblingDB('baz').dropDatabase();
  assert.eq(0, testDB.getSiblingDB('baz').bar.count());

  // Insert into the 'foo' database
  testDB.bar.insert({x: 1});
  testDB.bar.insert({x: 0});
  // and into the 'baz' database
  testDB.getSiblingDB('baz').bar.insert({x: 2});

  // Running mongodump with '--query' specified but no '--db' should fail
  var dumpArgs = ['dump',
    '--collection', 'bar',
    '--query', '{ "x": { "$gt":0 } }']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);
  assert(toolTest.runTool.apply(toolTest, dumpArgs) !== 0,
    'mongodump should exit with a non-zero status when --query is ' +
    'specified but --db isn\'t');

  // Running mongodump with '--queryFile' specified but no '--db' should fail
  dumpArgs = ['dump',
    '--collection', 'bar',
    '--queryFile', 'jstests/dump/testdata/query.json']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);
  assert(toolTest.runTool.apply(toolTest, dumpArgs) !== 0,
    'mongodump should exit with a non-zero status when --queryFile is ' +
    'specified but --db isn\'t');

  // Running mongodump with '--query' specified but no '--collection' should fail
  dumpArgs = ['dump',
    '--db', 'foo',
    '--query', '"{ "x": { "$gt":0 } }"']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);
  assert(toolTest.runTool.apply(toolTest, dumpArgs) !== 0,
    'mongodump should exit with a non-zero status when --query is ' +
    'specified but --collection isn\'t');

  // Running mongodump with '--queryFile' specified but no '--collection' should fail
  dumpArgs = ['dump',
    '--db', 'foo',
    '--queryFile', 'jstests/dump/testdata/query.json']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);
  assert(toolTest.runTool.apply(toolTest, dumpArgs) !== 0,
    'mongodump should exit with a non-zero status when --queryFile is ' +
    'specified but --collection isn\'t');


  // Running mongodump with a '--queryFile' that doesn't exist should fail
  dumpArgs = ['dump',
    '--collection', 'bar',
    '--db', 'foo',
    '--queryFile', 'jstests/nope']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);
  assert(toolTest.runTool.apply(toolTest, dumpArgs) !== 0,
    'mongodump should exit with a non-zero status when --queryFile doesn\'t exist');

  // Running mongodump with '--query' should only get matching documents
  resetDbpath(targetPath);
  dumpArgs = ['dump',
    '--query', '{ "x": { "$gt":0 } }',
    '--db', 'foo',
    '--collection', 'bar']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);
  assert.eq(toolTest.runTool.apply(toolTest, dumpArgs), 0,
    'mongodump should return exit status 0 when --db, --collection, and ' +
    '--query are all specified');

  var restoreTest = function() {
    testDB.dropDatabase();
    testDB.getSiblingDB('baz').dropDatabase();
    assert.eq(0, testDB.bar.count());
    assert.eq(0, testDB.getSiblingDB('baz').bar.count());

    var restoreArgs = ['restore'].
      concat(getRestoreTarget(targetPath)).
      concat(commonToolArgs);
    assert.eq(toolTest.runTool.apply(toolTest, restoreArgs), 0,
      'mongorestore should succeed');
    assert.eq(1, testDB.bar.count());
    assert.eq(0, testDB.getSiblingDB('baz').bar.count());
  };

  restoreTest();

  // Running mongodump with '--queryFile' should only get matching documents
  resetDbpath(targetPath);
  dumpArgs = ['dump',
    '--queryFile', 'jstests/dump/testdata/query.json',
    '--db', 'foo',
    '--collection', 'bar']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);
  assert.eq(toolTest.runTool.apply(toolTest, dumpArgs), 0,
    'mongodump should return exit status 0 when --db, --collection, and ' +
    '--query are all specified');

  restoreTest();

  toolTest.stop();
}());
