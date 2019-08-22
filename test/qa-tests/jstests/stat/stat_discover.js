(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }
  load("jstests/libs/output.js");

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
  const discovered = slaves[0];
  const specified = slaves[1];

  clearRawMongoProgramOutput();
  pid = startMongoProgramNoConnect("mongostat", "--host", specified.host, "--discover");

  assert.soon(hasPort(discovered.port), "discovered host (" + discovered.host + ") is seen");
  assert.soon(hasPort(specified.port), "specified host (" + specified.host + ") is seen");

  rs.stop(discovered);
  assert.soon(lacksPort(discovered.port), "after discovered host (" + discovered.host + ") is stopped, it is not seen");
  assert.soon(hasPort(specified.port), "after discovered host (" + discovered.host + ") is stopped, specified host (" + specified.host + ") is still seen");

  rs.start(discovered);
  assert.soon(hasPort(discovered.port), "after discovered host (" + discovered.host + ") is restarted, it is seen again");
  assert.soon(hasPort(specified.port), "after discovered host (" + discovered.host + ") is restarted, specified host (" + specified.host + ") is still seen");

  rs.stop(specified);
  assert.soon(lacksPort(specified.port), "after specified host (" + specified.host + ") is stopped, it is not seen");
  assert.soon(hasPort(discovered.port), "after specified host (" + specified.host + ") is stopped, the discovered host (" + discovered.host + ") is still seen");

  stopMongoProgramByPid(pid);

  rs.stopSet();
  toolTest.stop();
}());
