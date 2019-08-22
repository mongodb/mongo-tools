// Tests that mongodump and mongorestore properly handle access control information when doing
// single-db dumps and restores. Tests proper handling of dumping and restoring the admin database.
// This test is derived from legacy30 test dumprestore_auth3.js

(function () {
  // Runs the tool with the given name against the given mongod.
  function runTool(toolName, mongod, options) {
    const opts = {host: mongod.host};
    Object.extend(opts, options);
    MongoRunner.runMongoTool(toolName, opts);
  }

  const mongod = MongoRunner.runMongod();
  const admindb = mongod.getDB("admin");
  const foodb = mongod.getDB("foo");
  const bardb = mongod.getDB("bar");

  jsTestLog("Creating Admin user & initial data");
  admindb.createUser({user: 'root', pwd: 'pass', roles: ['root']});
  admindb.createUser({user: 'backup', pwd: 'pass', roles: ['backup']});
  admindb.createUser({user: 'restore', pwd: 'pass', roles: ['restore']});
  admindb.createRole({role: "dummyRole", roles: [], privileges: []});
  foodb.createUser({user: 'user', pwd: 'pass', roles: jsTest.basicUserRoles});
  foodb.createRole({role: 'role', roles: [], privileges: []});
  const backupActions = ['find'];
  foodb.createRole({
    role: 'backupFooChester',
    privileges: [{resource: {db: 'foo', collection: 'chester'}, actions: backupActions}],
    roles: []
  });
  foodb.createUser({user: 'backupFooChester', pwd: 'pass', roles: ['backupFooChester']});

  var userCount = foodb.getUsers().length;
  var rolesCount = foodb.getRoles().length;
  var adminUsersCount = admindb.getUsers().length;
  const adminRolesCount = admindb.getRoles().length;
  const systemUsersCount = admindb.system.users.count();
  const systemVersionCount = admindb.system.version.count();

  foodb.bar.insert({a: 1});

  assert.eq(1, foodb.bar.findOne().a);
  assert.eq(userCount, foodb.getUsers().length, "setup");
  assert.eq(rolesCount, foodb.getRoles().length, "setup2");
  assert.eq(adminUsersCount, admindb.getUsers().length, "setup3");
  assert.eq(adminRolesCount, admindb.getRoles().length, "setup4");
  assert.eq(systemUsersCount, admindb.system.users.count(), "setup5");
  assert.eq(systemVersionCount, admindb.system.version.count(), "system version");
  assert.eq(1, admindb.system.users.count({user: "restore"}), "Restore user is missing");
  assert.eq(1, admindb.system.users.count({user: "backup"}), "Backup user is missing");
  const versionDoc = admindb.system.version.findOne();

  jsTestLog("Dump foo database without dumping user data");
  const dumpDir = MongoRunner.getAndPrepareDumpDirectory("dumprestore_auth3");
  runTool("mongodump", mongod, {out: dumpDir, db: "foo"});

  foodb.dropDatabase();
  foodb.dropAllUsers();
  foodb.dropAllRoles();

  jsTestLog("Restore foo database from dump that doesn't contain user data ");

  runTool("mongorestore", mongod, {dir: dumpDir + "foo/", db: 'foo', restoreDbUsersAndRoles: ""});

  assert.soon(function () {
    return foodb.bar.findOne();
  }, "no data after restore");
  assert.eq(1, foodb.bar.findOne().a);
  assert.eq(0, foodb.getUsers().length, "Restore created users somehow");
  assert.eq(0, foodb.getRoles().length, "Restore created roles somehow");

  // Re-create user data
  foodb.createUser({user: 'user', pwd: 'password', roles: jsTest.basicUserRoles});
  foodb.createRole({role: 'role', roles: [], privileges: []});
  userCount = 1;
  rolesCount = 1;

  assert.eq(1, foodb.bar.findOne().a);
  assert.eq(userCount, foodb.getUsers().length, "didn't create user");
  assert.eq(rolesCount, foodb.getRoles().length, "didn't create role");

  jsTestLog("Dump foo database *with* user data");
  runTool("mongodump", mongod, {out: dumpDir, db: "foo", dumpDbUsersAndRoles: ""});

  foodb.dropDatabase();
  foodb.dropAllUsers();
  foodb.dropAllRoles();

  assert.eq(0, foodb.getUsers().length, "didn't drop users");
  assert.eq(0, foodb.getRoles().length, "didn't drop roles");
  assert.eq(0, foodb.bar.count(), "didn't drop 'bar' collection");

  jsTestLog("Restore foo database without restoring user data, even though it's in the dump");
  runTool("mongorestore", mongod, {dir: dumpDir + "foo/", db: 'foo'});

  assert.soon(function () {
    return foodb.bar.findOne();
  }, "no data after restore");
  assert.eq(1, foodb.bar.findOne().a);
  assert.eq(0, foodb.getUsers().length, "Restored users even though it shouldn't have");
  assert.eq(0, foodb.getRoles().length, "Restored roles even though it shouldn't have");

  jsTestLog("Restore foo database *with* user data");
  runTool("mongorestore", mongod, {dir: dumpDir + "foo/", db: 'foo', restoreDbUsersAndRoles: ""});

  assert.soon(function () {
    return foodb.bar.findOne();
  }, "no data after restore");
  assert.eq(1, foodb.bar.findOne().a);
  assert.eq(userCount, foodb.getUsers().length, "didn't restore users");
  assert.eq(rolesCount, foodb.getRoles().length, "didn't restore roles");
  assert.eq(1, admindb.system.users.count({user: "restore", db: "admin"}), "Restore user is missing");
  assert.docEq(versionDoc, admindb.system.version.findOne(), "version doc was changed by restore");

  jsTestLog("Make modifications to user data that should be overridden by the restore");
  foodb.dropUser('user');
  foodb.createUser({user: 'user2', pwd: 'password2', roles: jsTest.basicUserRoles});
  foodb.dropRole('role');
  foodb.createRole({role: 'role2', roles: [], privileges: []});

  jsTestLog("Restore foo database (and user data) with --drop so it overrides the changes made");

  // Restore with --drop to override the changes to user data
  runTool("mongorestore", mongod,
    {dir: dumpDir + "foo/", db: 'foo', drop: "", restoreDbUsersAndRoles: ""});

  assert.soon(function () {
    return foodb.bar.findOne();
  }, "no data after restore");
  assert.eq(adminUsersCount, admindb.getUsers().length, "Admin users were dropped");
  assert.eq(adminRolesCount, admindb.getRoles().length, "Admin roles were dropped");
  assert.eq(1, foodb.bar.findOne().a);
  assert.eq(userCount, foodb.getUsers().length, "didn't restore users");
  assert.eq("user", foodb.getUser('user').user, "didn't update user");
  assert.eq(rolesCount, foodb.getRoles().length, "didn't restore roles");
  assert.eq("role", foodb.getRole('role').role, "didn't update role");
  assert.docEq(versionDoc, admindb.system.version.findOne(), "version doc was changed by restore");

  jsTestLog("Dump just the admin database.  User data should be dumped by default");
  // Make a user in another database to make sure it is properly captured
  bardb.createUser({user: "user", pwd: 'pwd', roles: []});
  admindb.createUser({user: "user", pwd: 'pwd', roles: []});
  adminUsersCount += 1;
  runTool("mongodump", mongod, {out: dumpDir, db: "admin"});

  // Change user data a bit.
  foodb.dropAllUsers();
  bardb.createUser({user: "user2", pwd: 'pwd', roles: []});
  admindb.dropAllUsers();

  jsTestLog("Restore just the admin database. User data should be restored by default");
  runTool("mongorestore", mongod, {dir: dumpDir + "admin/", db: 'admin', drop: ""});

  assert.soon(function () {
    return foodb.bar.findOne();
  }, "no data after restore");
  assert.eq(1, foodb.bar.findOne().a);
  assert.eq(userCount, foodb.getUsers().length, "didn't restore users");
  assert.eq("user", foodb.getUser('user').user, "didn't restore user");
  assert.eq(rolesCount, foodb.getRoles().length, "didn't restore roles");
  assert.eq("role", foodb.getRole('role').role, "didn't restore role");
  assert.eq(1, bardb.getUsers().length, "didn't restore users for bar database");
  assert.eq("user", bardb.getUsers()[0].user, "didn't restore user for bar database");
  assert.eq(adminUsersCount, admindb.getUsers().length, "didn't restore users for admin database");
  assert.eq("user", admindb.getUser("user").user, "didn't restore user for admin database");
  assert.eq(6, admindb.system.users.count(), "has the wrong # of users for the whole server");
  assert.eq(2, admindb.system.roles.count(), "has the wrong # of roles for the whole server");
  assert.docEq(versionDoc, admindb.system.version.findOne(), "version doc was changed by restore");

  jsTestLog("Dump all databases");
  runTool("mongodump", mongod, {out: dumpDir});

  foodb.dropDatabase();
  foodb.dropAllUsers();
  foodb.dropAllRoles();

  assert.eq(0, foodb.getUsers().length, "didn't drop users");
  assert.eq(0, foodb.getRoles().length, "didn't drop roles");
  assert.eq(0, foodb.bar.count(), "didn't drop 'bar' collection");

  jsTestLog("Restore all databases");
  runTool("mongorestore", mongod, {dir: dumpDir});

  assert.soon(function () {
    return foodb.bar.findOne();
  }, "no data after restore");
  assert.eq(1, foodb.bar.findOne().a);
  assert.eq(1, foodb.getUsers().length, "didn't restore users");
  assert.eq(1, foodb.getRoles().length, "didn't restore roles");
  assert.docEq(versionDoc, admindb.system.version.findOne(), "version doc was changed by restore");

  MongoRunner.stopMongod(mongod);
}());
