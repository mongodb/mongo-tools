(function() {

    if (typeof getToolTest === 'undefined') {
        load('jstests/configs/sharding_28.config.js');
    }


   if (dump_targets == "archive") {
       print('skipping test incompatable with archiving');
       return assert(true);
   }

    var toolTest = getToolTest('fullrestore');
    var commonToolArgs = getCommonToolArguments();

    var sourceDB = toolTest.db.getSiblingDB('blahblah');

    // put in some sample data
    for(var i=0;i<100;i++){
      sourceDB.test.insert({x:1})
    }

    // dump the data
    var ret = toolTest.runTool.apply( toolTest, ['dump'].
            concat(getDumpTarget()).
            concat(commonToolArgs));
    assert.eq(ret, 0, "dump of full sharded system should have succeeded")

    // a full restore should fail
    var ret = toolTest.runTool.apply( toolTest, ['restore'].
            concat(getRestoreTarget()).
            concat(commonToolArgs));
    assert.neq(ret, 0, "restore of full sharded system should have failed")
    
    // delete the config dir
    resetDbpath("dump/config")

    // *now* the restore should succeed
    var ret = toolTest.runTool.apply( toolTest, ['restore'].
            concat(getRestoreTarget()).
            concat(commonToolArgs));
    assert.eq(ret, 0, "restore of sharded system without config db should have succeeded")

}());
