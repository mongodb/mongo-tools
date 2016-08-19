(function() {
  jsTest.log('Testing that dump utilizes read preferences and tags');
  var toolTest = new ToolTest('dump_preference_and_tags');

  var replset1 = new ReplSetTest({nodes: 3, name: 'replset'});
  replset1.startSet();
  replset1.initiate();

  var primary = replset1.getPrimary();
  var secondaries = replset1.getSecondaries();

  // rs functions actually operate on db
  db = primary.getDB('foo'); // eslint-disable-line no-native-reassign

  db.bar.insertOne({}, {writeConcern: {w: 3}});

  secondaries.forEach(function(secondary) {
    secondary.getDB('foo').setProfilingLevel(2);
  });
  primary.getDB('foo').setProfilingLevel(2);

  var conf = rs.conf();

  var hostByTag = {};
  var i = 1;
  conf.members.forEach(function(member) {
    if (member.host === primary.host) {
      member.tags = {use: "primary"};
    } else {
      member.tags = {use: "secondary" + i};
      hostByTag["secondary"+i]=member.host;
      i++;
    }
  });

  rs.reconfig(conf);

  runMongoProgram('mongodump', '--host', "replset/"+primary.host, '--readPreference={mode:"nearest", tags:{use:"secondary1"}}');

  replset1.nodes.forEach(function(node) {
    var count = node.getDB('foo').system.profile.find().count();
    jsTest.log(node.host+" "+count);
    if (node.host === hostByTag.secondary1) {
      assert.neq(count, 0, node.host);
    } else {
      assert.eq(count, 0, node.host);
    }
  });

  toolTest.stop();
}());
