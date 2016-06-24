(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }

  // only run this test for wiredTiger
  if (TestData && TestData.storageEngine !== 'wiredTiger') {
    return;
  }

  var t = getToolTest("OptionsJSON");
  var testDB = t.db.getSiblingDB('foo');
  testDB.dropDatabase();
  testDB.createCollection("testcoll", {storageEngine: {wiredTiger: {"configString": "block_compressor=snappy"}}});

  // full dump/restore should work
  ret = t.runTool("dump", "--out", t.ext);
  assert.eq(0, ret);
  testDB.dropDatabase();

  ret = t.runTool('restore', t.ext);
  assert.eq(0, ret);

  t.stop();
}());
