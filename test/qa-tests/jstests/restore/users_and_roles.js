(function() {

    if (typeof getToolTest === 'undefined') {
        load('jstests/configs/plain_28.config.js');
    }

    // Tests running mongorestore with --restoreDbUsersAndRoles

    jsTest.log('Testing running mongorestore with --restoreDbUsersAndRoles');

    var toolTest = getToolTest('users_and_roles');
    var commonToolArgs = getCommonToolArguments();

    // where we'll put the dump
    var dumpTarget = 'users_and_roles_dump';
    resetDbpath(dumpTarget);

    // the db we'll be using 
    var testDB = toolTest.db.getSiblingDB('test');

    // create some users and roles on the database
    testDB.createUser(
        {
            user: 'userOne',
            pwd: 'pwdOne',
            roles: [
                { role: 'read', db: 'test' },
            ],
        }
    );
    testDB.createRole(
        {
            role: 'roleOne',
            privileges: [
                { 
                    resource: { db: 'test', collection: '' }, 
                    actions: ['find'],
                },
            ],
            roles: [],
        }
    );
    testDB.createUser(
        {
            user: 'userTwo',
            pwd: 'pwdTwo',
            roles: [
                { role: 'roleOne', db: 'test' },
            ],
        }
    );

    // insert some data
    for (var i = 0; i < 10; i++) {
        testDB.data.insert({ _id: i });
    }
    // sanity check the insertion worked
    assert.eq(10, testDB.data.count());

    // dump the data
    var ret = toolTest.runTool.apply(
        toolTest,    
        ['dump', '--out', dumpTarget, '--db', 'test', '--dumpDbUsersAndRoles'].
            concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // drop the database, users, and roles
    testDB.dropDatabase();
    testDB.dropAllUsers();
    testDB.dropAllRoles();

    // restore the data, specifying --restoreDBUsersAndRoles
    ret = toolTest.runTool.apply(
        toolTest,
        ['restore', '--db', 'test', '--restoreDbUsersAndRoles', dumpTarget+'/test'].
            concat(commonToolArgs)
    );
    assert.eq(0, ret);

    // make sure the data was restored
    assert.eq(10, testDB.data.count());
    for (var i = 0; i < 10; i++) {
        assert.eq(1, testDB.data.count({ _id: i }));
    }

    // make sure the users were restored
    var users = testDB.getUsers();
    assert.eq(2, users.length);
    assert(users[0].user === 'userOne' || users[1].user === 'userOne');
    assert(users[0].user === 'userTwo' || users[1].user === 'userTwo');

    // make sure the role was restored
    var roles = testDB.getRoles();
    assert.eq(1, roles.length);
    assert.eq('roleOne', roles[0].role);

    // success
    toolTest.stop();


}());
