(function() {

  if (typeof getToolTest === "undefined") {
    load('jstests/configs/plain_28.config.js');
  }

  // Tests running mongoexport exporting to json with the --projection option

  jsTest.log('Testing exporting to json using the --projection option');

  var toolTest = getToolTest('projection_json');
  var commonToolArgs = getCommonToolArguments();

  // the db and collections we'll use
  var testDB = toolTest.db.getSiblingDB('test');
  var sourceColl = testDB.source;
  var destColl = testDB.dest;

  // the export target
  var exportTarget = 'projection_json.json';
  removeFile(exportTarget);

  // insert some data
  sourceColl.insert({a: 1});
  sourceColl.insert({a: 1, b: 1});
  sourceColl.insert({a: 1, b: 2, c: 3});
  // sanity check the insertion worked
  assert.eq(3, sourceColl.count());

  // export the data, specifying only one projection
  var ret = toolTest.runTool.apply(toolTest, ['export',
    '--out', exportTarget,
    '--db', 'test',
    '--collection', 'source',
    '--projection', '{"a":1,"b":0}']
    .concat(commonToolArgs));
  assert.eq(0, ret);

  // import the data into the destination collection
  ret = toolTest.runTool.apply(toolTest, ['import',
    '--file', exportTarget,
    '--db', 'test',
    '--collection', 'dest',
    '--type', 'json']
    .concat(commonToolArgs));
  assert.eq(0, ret);

  // make sure only the specified projection was exported
  assert.eq(3, destColl.count({a: 1}));
  assert.eq(0, destColl.count({b: 1}));
  assert.eq(0, destColl.count({b: 2}));
  assert.eq(0, destColl.count({c: 3}));

  // remove the export, clear the destination collection
  removeFile(exportTarget);
  destColl.remove({});

  // export the data, specifying all projections
  ret = toolTest.runTool.apply(toolTest, ['export',
    '--out', exportTarget,
    '--db', 'test',
    '--collection', 'source',
    '--projection', '{"a":1,"b":1,"c":1}']
    .concat(commonToolArgs));
  assert.eq(0, ret);

  // import the data into the destination collection
  ret = toolTest.runTool.apply(toolTest, ['import',
    '--file', exportTarget,
    '--db', 'test',
    '--collection', 'dest',
    '--type', 'json']
    .concat(commonToolArgs));
  assert.eq(0, ret);

  // make sure everything was exported
  assert.eq(3, destColl.count({a: 1}));
  assert.eq(1, destColl.count({b: 1}));
  assert.eq(1, destColl.count({b: 2}));
  assert.eq(1, destColl.count({c: 3}));

  // make sure the _id was exported - the _id for the
  // corresponding documents in the two collections should
  // be the same
  var fromSource = sourceColl.findOne({a: 1, b: 1});
  var fromDest = destColl.findOne({a: 1, b: 1});
  assert.eq(fromSource._id, fromDest._id);

  // success
  toolTest.stop();

}());
