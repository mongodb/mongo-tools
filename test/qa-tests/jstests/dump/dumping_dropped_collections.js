if (typeof getToolTest === 'undefined') {
  load('jstests/configs/plain_28.config.js');
}

// Create a number of collections, then simultaneously drop them and dump them

// it would be nice to verify that dump has emitted the
// "the collection foo.bar appears to have been dropped after the dump started"
// log line, but by the time dump finishes those lines have pushed off of the top
// of the captured output buffer

(function() {
  var targetPath = "dump_dumping_dropped_collections_test";
  resetDbpath(targetPath);
  var toolTest = getToolTest('DumpingDroppedCollectionsTest');
  var commonToolArgs = getCommonToolArguments();
  testDB = toolTest.db.getSiblingDB('foo');

  // create
  testDB.dropDatabase();
  for (var i=0; i<1000; i++) {
    print("creating bar_"+i);
    testDB.getCollection("bar_"+i).insert({x: i});
  }

  // drop
  startParallelShell(
    // sleep here so that we don't start dropping collections until after the dump
    // has retrieved the catalog of collections to dump
    'sleep(200); '+
    'print(\'dropping collections\'); '+
    'for (var i=0;i<1000;i++) {'+
    '  print("dropping bar_"+i);'+
    '  db.getCollection("bar_"+i).drop(); '+
    '}'
  );

  // dump
  var dumpArgs = ['dump'].concat(getDumpTarget(targetPath)).concat(commonToolArgs);

  assert(toolTest.runTool.apply(toolTest, dumpArgs) === 0, 'mongodump should not crash when dumping collections that gets dropped');

  toolTest.stop();
}());
