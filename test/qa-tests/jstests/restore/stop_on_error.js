(function() {

    if (typeof getToolTest === 'undefined') {
        load('jstests/configs/plain_28.config.js');
    }

    var toolTest = getToolTest('stop_on_error');
    var commonToolArgs = getCommonToolArguments();

    var dbOne = toolTest.db.getSiblingDB('dbOne');
    // create a test collection
    for(var i=0;i<=100;i++){
      dbOne.test.insert({_id:i, x:i*i})
    }

    // dump it
    var dumpTarget = 'stop_on_error_dump';
    resetDbpath(dumpTarget);
    var ret = toolTest.runTool.apply(
        toolTest,
        ['dump', '--out', dumpTarget].
            concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // drop the database so it's empty
    dbOne.dropDatabase()

    // restore it - database was just dropped, so this should work successfully
    ret = toolTest.runTool.apply(
        toolTest,    
        ['restore', dumpTarget].
            concat(commonToolArgs)
    );
    assert.eq(0, ret, "restore to empty DB should have returned successfully");

    // restore it again with --stopOnError - this one should fail since there are dup keys
    ret = toolTest.runTool.apply(
        toolTest,    
        ['restore', dumpTarget, '--stopOnError', '-vvvv'].
            concat(commonToolArgs)
    );
    assert.neq(0, ret);

    // restore it one more time without --stopOnError - there are dup keys but they will be ignored
    ret = toolTest.runTool.apply(
        toolTest,    
        ['restore', dumpTarget, '-vvvv'].
            concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // success
    toolTest.stop();

}());
