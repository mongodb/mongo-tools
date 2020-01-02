(function() {

  if (typeof getToolTest === "undefined") {
    load('jstests/configs/plain_28.config.js');
  }

  // Tests running mongoexport without --forceTableScan specified, but on a wired tiger collection.

  jsTest.log('Testing exporting without --forceTableScan on a wired tiger collection');

  var toolTest = getToolTest('wired_tiger_table_scan');
  var commonToolArgs = getCommonToolArguments();

  // the export target
  var exportTarget = 'wired_tiger_table_scan_export.json';
  removeFile(exportTarget);

  // the db and collection we'll use
  var testDB = toolTest.db.getSiblingDB('test');
  var testColl = testDB.data;

  // insert some data
  var data = [];
  for (var i = 0; i < 50; i++) {
    data.push({_id: i});
  }
  testColl.insertMany(data);
  // sanity check the insertion worked
  assert.eq(50, testColl.count());

  // set the profiling level to high, so that
  // we can inspect all queries
  assert.eq(1, testDB.setProfilingLevel(2).ok);

  // the profiling collection
  var profilingColl = testDB.system.profile;

  // run mongoexport without --forceTableScan
  var ret = toolTest.runTool.apply(toolTest, ['export',
    '--out', exportTarget,
    '--db', 'test',
    '--collection', 'data']
    .concat(commonToolArgs));
  assert.eq(0, ret);

  // grab the query from the profiling collection
  var queries = profilingColl.find({op: 'query', ns: 'test.data'}).toArray();

  // there should only be one query so far, and it should have snapshot set (or equivalent).
  assert.eq(1, queries.length);
  if (queries[0].command === undefined) {
    assert.eq(true, queries[0].query.$snapshot || queries[0].query.snapshot || queries[0].query.hint._id);
  } else {
    assert.eq(true, queries[0].command.snapshot || queries[0].command.hint._id === 1);
  }

  // remove the export file
  removeFile(exportTarget);

  // run mongoexport again, with --forceTableScan
  ret = toolTest.runTool.apply(toolTest, ['export',
    '--out', exportTarget,
    '--db', 'test',
    '--collection', 'data']
    .concat(commonToolArgs));
  assert.eq(0, ret);

  // grab the queries again
  queries = profilingColl.find({op: 'query', ns: 'test.data'}).sort({ts: 1}).toArray();

  // there should be two queries, and the second one should not have snapshot set (or equivalent).
  assert.eq(2, queries.length);
  if (queries[1].command === undefined) {
    assert(!queries[1].query['$snapshot'] && !queries[1].query.hint);
  } else {
    assert.eq(true, !queries[1].command.snapshot && !queries[1].command.hint);
  }


  // wipe the collection
  testColl.remove({});

  // import the data back in
  ret = toolTest.runTool.apply(toolTest, ['import',
    '--file', exportTarget,
    '--db', 'test',
    '--collection', 'data']
    .concat(commonToolArgs));
  assert.eq(0, ret);

  // make sure that the export exported the correct data
  assert.eq(50, testColl.count());
  for (i = 0; i < 50; i++) {
    assert.eq(1, testColl.count({_id: i}));
  }

  // remove the export file
  removeFile(exportTarget);

  // success
  toolTest.stop();

}());
