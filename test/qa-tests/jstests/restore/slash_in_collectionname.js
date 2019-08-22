// Tests using mongodump/mongorestore to dump/restore data from/to a collection with a slash in its name

(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }

  jsTest.log('Testing restoration of a collection with a slash in its name');

  if (dump_targets === 'archive') {
    jsTest.log('Skipping test unsupported against archive targets');
    return assert(true);
  }

  var toolTest = getToolTest('slash_in_collectionname');
  var commonToolArgs = getCommonToolArguments();

  // where we'll put the dump
  var dumpTarget = 'slash_in_collectionname_dump';
  resetDbpath(dumpTarget);

  // the db we will dump from
  var sourceDB = toolTest.db.getSiblingDB('source');
  // the collection we will dump from
  var sourceCollName = 'source/Coll';

  // insert a bunch of data
  var data = [];
  for (var i = 0; i < 500; i++) {
    data.push({_id: i});
  }
  sourceDB[sourceCollName].insertMany(data);
  // sanity check the insertion worked
  assert.eq(500, sourceDB[sourceCollName].count());

  // dump the data
  var ret = toolTest.runTool.apply(toolTest, ['dump'].concat(getDumpTarget(dumpTarget)));
  assert.eq(0, ret);

  sourceDB[sourceCollName].drop();

  // restore the data
  ret = toolTest.runTool.apply(toolTest, ['restore']
    .concat(getRestoreTarget(dumpTarget))
    .concat(commonToolArgs));
  assert.eq(0, ret);

  // make sure the data was restored correctly
  assert.eq(500, sourceDB[sourceCollName].count());
  for (i = 0; i < 500; i++) {
    assert.eq(1, sourceDB[sourceCollName].count({_id: i}));
  }

  // success
  toolTest.stop();
}());
