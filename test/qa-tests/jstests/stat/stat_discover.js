(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }
  load("jstests/libs/mongostat.js");

  var toolTest = getToolTest("stat_discover");
  var rs = new ReplSetTest({
    name: "rpls",
    nodes: 4,
    useHostName: true,
  });

  rs.startSet();
  rs.initiate();
  rs.awaitReplication();

  if ("liveNodes" in rs) {
    // MongoDB < 4.0
    master = rs.liveNodes.master;
    slaves = rs.liveNodes.slaves;
  } else {
    // MongoDB >= 4.0
    master = rs._master;
    slaves = rs._slaves;
  }


  worked = statCheck(["mongostat",
    "--port", master.port,
    "--discover"],
  hasOnlyPorts(rs.ports));
  assert(worked, "when only port is used, each host still only appears once");

  assert(discoverTest(rs.ports, master.host), "--discover against a replset master sees all members");

  assert(discoverTest(rs.ports, slaves[0].host), "--discover against a replset slave sees all members");

  hosts = [master.host, slaves[0].host, slaves[1].host];
  ports = [master.port, slaves[0].port, slaves[1].port];
  worked = statCheck(['mongostat',
    '--host', hosts.join(',')],
  hasOnlyPorts(ports));
  assert(worked, "replica set specifiers are correctly used");

  assert(discoverTest([toolTest.port], toolTest.m.host), "--discover against a stand alone-sees just the stand-alone");

  // Test discovery with nodes cutting in and out
  clearRawMongoProgramOutput();
  pid = startMongoProgramNoConnect("mongostat", "--host", slaves[1].host, "--discover");

  assert.soon(hasPort(slaves[0].port), "discovered host is seen");
  assert.soon(hasPort(slaves[1].port), "specified host is seen");

  rs.stop(slaves[0]);
  assert.soon(lacksPort(slaves[0].port), "after discovered host is stopped, it is not seen");
  assert.soon(hasPort(slaves[1].port), "after discovered host is stopped, specified host is still seen");

  rs.start(slaves[0]);
  assert.soon(hasPort(slaves[0].port), "after discovered is restarted, discovered host is seen again");
  assert.soon(hasPort(slaves[1].port), "after discovered is restarted, specified host is still seen");

  rs.stop(slaves[1]);
  assert.soon(lacksPort(slaves[1].port), "after specified host is stopped, specified host is not seen");
  assert.soon(hasPort(slaves[0].port), "after specified host is stopped, the discovered host is still seen");

  stopMongoProgramByPid(pid);

  rs.stopSet();
  toolTest.stop();
}());
