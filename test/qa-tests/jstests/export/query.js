(function() {

    if (typeof getToolTest === 'undefined') {
        load('jstests/configs/plain_28.config.js');
    }

    // Tests running mongoexport with --query specified.
    
    jsTest.log('Testing exporting with --query'); 

    var toolTest = getToolTest('query');
    var commonToolArgs = getCommonToolArguments();

    // the export target
    var exportTarget = 'query_export.json';
    removeFile(exportTarget);

    // the db and collections we'll use
    var testDB = toolTest.db.getSiblingDB('test');
    var sourceColl = testDB.source;
    var destColl = testDB.dest;

    // insert some data
    sourceColl.insert({ a: 1, x: { b: '1' }});
    sourceColl.insert({ a: 2, x: { b: '1', c: '2' } });
    sourceColl.insert({ a: 1, c: '1' });
    sourceColl.insert({ a: 2, c: '2' });
    // sanity check the insertion worked
    assert.eq(4, sourceColl.count());

    // export the data, with a query that will match nothing
    var ret = toolTest.runTool.apply(
        toolTest,
        ['export', '--out', exportTarget, '--db', 'test',
            '--collection', 'source', '--query', '{a:3}'].
            concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // import the data into the destination collection
    ret = toolTest.runTool.apply(
        toolTest,
        ['import', '--file', exportTarget, '--db', 'test',
            '--collection', 'dest'].
            concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // make sure the export was blank
    assert.eq(0, destColl.count());

    // remove the export
    removeFile(exportTarget);

    // export the data, with a query matching a single element
    ret = toolTest.runTool.apply(
        toolTest,
        ['export', '--out', exportTarget, '--db', 'test',
            '--collection', 'source', '--query', "{a:1, c:'1'}"].
            concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // import the data into the destination collection 
    ret = toolTest.runTool.apply(
        toolTest,
        ['import', '--file', exportTarget, '--db', 'test',
            '--collection', 'dest'].
            concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // make sure the query was applied correctly
    assert.eq(1, destColl.count());
    assert.eq(1, destColl.count({ a: 1, c: '1' })); 

    // remove the export, clear the destination collection
    removeFile(exportTarget);
    destColl.remove({});

    // export the data, with a query on an embedded document
    ret = toolTest.runTool.apply(
        toolTest,
        ['export', '--out', exportTarget, '--db', 'test',
            '--collection', 'source', '--query', "{a:2, 'x.c':'2'}"].
            concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // import the data into the destination collection 
    ret = toolTest.runTool.apply(
        toolTest,
        ['import', '--file', exportTarget, '--db', 'test',
            '--collection', 'dest'].
            concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // make sure the query was applied correctly
    assert.eq(1, destColl.count());
    assert.eq(1, destColl.count({ a: 2, x: { b: '1', c: '2' } }));

    // remove the export, clear the destination collection
    removeFile(exportTarget);
    destColl.remove({});

    // export the data, with a blank query (should match everything)
    ret = toolTest.runTool.apply(
        toolTest,
        ['export', '--out', exportTarget, '--db', 'test',
            '--collection', 'source', '--query', "{}"].
            concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // import the data into the destination collection 
    ret = toolTest.runTool.apply(
        toolTest,
        ['import', '--file', exportTarget, '--db', 'test',
            '--collection', 'dest'].
            concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // make sure the query was applied correctly
    assert.eq(4, destColl.count());

    // success
    toolTest.stop();
    
}());
