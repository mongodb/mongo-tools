(function() {

    if (typeof getToolTest === 'undefined') {
        load('jstests/configs/plain_28.config.js');
    }
    
    // Tests that mongorestore handles restoring different types of 
    // indexes correctly.
    
    jsTest.log('Testing restoration of different types of indexes'); 

    var toolTest = getToolTest('indexes');
    var commonToolArgs = getCommonToolArguments();

    // where we'll put the dump
    var dumpTarget = 'indexes_dump';

    // the db and collection we will use
    var testDB = toolTest.db.getSiblingDB('test');
    var testColl = testDB.coll;

    // create a bunch of indexes of different types
    testColl.ensureIndex({ a: 1 });
    testColl.ensureIndex({ b: 1 }, { sparse: true, unique: true });
    testColl.ensureIndex({ a: 1, b: -1 });
    testColl.ensureIndex({ listField: 1 });
    testColl.ensureIndex({ textField: 'text' }, { language: 'spanish' });
    testColl.ensureIndex({ geoField: '2dsphere' });
    
    // store the getIndexes() output, to compare with the output 
    // after dumping and restoring
    var indexesPre = testColl.getIndexes();

    // insert some data
    for (var i = 0; i < 5; i++) {
        testColl.insert({ a: i, b: i+1, listField: [i, i+1] });
        testColl.insert({ textField: 'hola '+i });
        testColl.insert({ geoField: { type: 'Point', coordinates: [i, i+1] }});
    }
    // sanity check the data was inserted
    assert.eq(15, testColl.count());
    
    // dump the data
    var ret = toolTest.runTool.apply(
            toolTest, 
            ['dump', '--out', dumpTarget].
                concat(commonToolArgs)
    );
    assert.eq(0, ret);
    
    // drop the collection
    testColl.drop();
    // sanity check that the drop worked
    assert.eq(0, testColl.count());
    assert.eq(0, testColl.getIndexes().length);

    // restore the data
    ret = toolTest.runTool.apply(
            toolTest,
            ['restore', dumpTarget].
                concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // make sure the data was restored correctly
    assert.eq(15, testColl.count());

    // make sure the indexes were restored correctly
    var indexesPost = testColl.getIndexes();
    assert.eq(indexesPre.length, indexesPost.length);

    // success
    toolTest.stop();

}());
