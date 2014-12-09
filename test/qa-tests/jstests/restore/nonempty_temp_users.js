(function() {

    // Tests running mongorestore and restoring users with a nonempty temp 
    // users collection.

    jsTest.log('Testing restoring users with a nonempty temp users collection.'+
        ' The restore should fail');

    var toolTest = new ToolTest('nonempty_temp_users');
    toolTest.startDB('foo');

    // where we'll put the dump
    var dumpTarget = 'nonempty_temp_users_dump';
    resetDbpath(dumpTarget);

    // the admin db
    var adminDB = toolTest.db.getSiblingDB('admin');

    // create a user on the admin database
    adminDB.createUser(
        {
            user: 'adminUser',
            pwd: 'password',
            roles: [
                { role: 'read', db: 'admin' },
            ],
        }
    );

    // dump the data
    var ret = toolTest.runTool('dump', '--out', dumpTarget);

    // clear out the user
    adminDB.dropAllUsers();

    // insert into the tempusers collection
    adminDB.tempusers.insert({ _id: 'corruption' });

    // restore the data. it should fail
    ret = toolTest.runTool('restore', dumpTarget);
    assert.neq(0, ret);

    // success
    toolTest.stop();

}());
