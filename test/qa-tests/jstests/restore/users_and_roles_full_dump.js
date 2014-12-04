(function() {

    // Tests running mongorestore with  --restoreDbUsersAndRoles against 
    // a full dump.
    
    jsTest.log('Testing running mongorestore with --restoreDbUsersAndRoles against'+
        ' a full dump');

    var runTest = function(sourceDBVersion, dumpVersion, restoreVersion, destDBVersion) {

        jsTest.log('Running with sourceDBVersion=' + (sourceDBVersion || 'latest') +
                ', dumpVersion=' + (dumpVersion || 'latest') + ', restoreVersion=' +
                (restoreVersion || 'latest') + ', and destDBVersion=' + 
                (destDBVersion || 'latest'));

        var toolTest = new ToolTest('users_and_roles_full_dump',
            { binVersion: sourceDBVersion });
        toolTest.startDB('foo');

        // where we'll put the dump
        var dumpTarget = 'users_and_roles_full_dump_dump';

        // the db we'll be using, and the admin db
        var adminDB = toolTest.db.getSiblingDB('admin');
        var testDB = toolTest.db.getSiblingDB('test');

        // create a user and role on the admin database
        adminDB.createUser(
            {
                user: 'adminUser',
                pwd: 'password',
                roles: [
                    { role: 'read', db: 'admin' },  
                ],
            }
        );
        adminDB.createRole(
            {
                role: 'adminRole',
                privileges: [
                    { 
                        resource: { db: 'admin', collection: '' }, 
                        actions: ['find'],
                    },
                ],
                roles: [],
            }
        );

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
        var ret = toolTest.runTool('dump' + (dumpVersion ? ('-'+dumpVersion) : ''),
                '--out', dumpTarget);
        assert.eq(0, ret);

        // restart the mongod, with a clean db path
        stopMongod(toolTest.port);
        resetDbpath(toolTest.dbpath);
        toolTest.m = null;
        toolTest.db = null;
        toolTest.options.binVersion = destDBVersion;
        toolTest.startDB('foo');

        // refresh the db references
        adminDB = toolTest.db.getSiblingDB('admin');
        testDB = toolTest.db.getSiblingDB('test');

        // do a full restore
        ret = toolTest.runTool('restore' + (restoreVersion ? ('-'+restoreVersion) : ''), 
            dumpTarget);
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
        var adminUsers = adminDB.getUsers();
        assert.eq(1, adminUsers.length);
        assert.eq('adminUser', adminUsers[0].user);

        // make sure the roles were restored
        var roles = testDB.getRoles();
        assert.eq(1, roles.length);
        assert.eq('roleOne', roles[0].role);
        var adminRoles = adminDB.getRoles();
        assert.eq(1, adminRoles.length);
        assert.eq('adminRole', adminRoles[0].role);

        // success
        toolTest.stop();
        
    }

    // 'undefined' triggers latest
    runTest('2.6', '2.6', undefined, '2.6');
    runTest('2.6', '2.6', undefined, undefined);
    runTest('2.6', undefined, undefined, undefined);
    runTest(undefined, '2.6', undefined, '2.6');
    runTest(undefined, undefined, undefined, '2.6');
    runTest(undefined, undefined, undefined, undefined);

}());
