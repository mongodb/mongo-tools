(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;

  var targetPath = 'dump_version_test.archive';
  var toolTest = getToolTest('versionTest');
  var commonToolArgs = getCommonToolArguments();
  var db = toolTest.db.getSiblingDB('foo');

  db.dropDatabase();
  assert.eq(0, db.bar.count());

  db.bar.insert({x: 1});

  var dumpArgs = ['dump',
    '--db', 'foo',
    '--archive='+targetPath]
    .concat(commonToolArgs);
  assert.eq(toolTest.runTool.apply(toolTest, dumpArgs), 0,
    'mongodump should succeed');

  db.dropDatabase();

  clearRawMongoProgramOutput();

  var restoreArgs = ['restore',
    '--archive='+targetPath, '-vvv']
    .concat(commonToolArgs);
  assert.eq(toolTest.runTool.apply(toolTest, restoreArgs), 0,
    'mongorestore should succeed');

  var out = rawMongoProgramOutput();
  assert.lte.soon(3, function() {
    out = rawMongoProgramOutput();
    return (out.match(/archive \w+ version/g) || []).length;
  }, "should see at least three version string in the output");
  assert(/archive format version "\S+"/.test(out), "format version found");
  assert(/archive server version "\S+"/.test(out), "server version found");
  assert(/archive tool version "\S+"/.test(out), "tool version found");

  toolTest.stop();
}());
