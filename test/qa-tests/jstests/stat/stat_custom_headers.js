(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }
  load("jstests/libs/mongostat.js");

  var toolTest = getToolTest("stat_custom_headers");
  var port = toolTest.port;

  var x, rows;
  x = runMongoProgram("mongostat", "--port", port,
      "-o", "host,conn,time", "-O", "metrics.record.moves");
  assert.eq(x, exitCodeBadOptions, "mongostat should fail with both -o and -O options");
  clearRawMongoProgramOutput();

  // basic -o
  x = runMongoProgram("mongostat", "--port", port,
      "-o", "host,conn,time", "-n", 4);
  assert.eq(x, 0, "mongostat should succeed with -o and -n options");
  rows = statRows();
  assert.eq(rows.length, 5, "expected 5 rows in mongostat output");
  assert.eq(statFields(rows[0]).join(), "host,conn,time",
      "first row doesn't match 'host conn time'");
  assert.eq(statFields(rows[1]).length, 3,
      "there should be exactly three entries for a row of this stat output");
  clearRawMongoProgramOutput();

  // basic -O
  x = runMongoProgram("mongostat", "--port", port,
      "-O", "host", "-n", 4);
  assert.eq(x, 0, "mongostat should succeed with -o and -n options");
  rows = statRows();
  var fields = statFields(rows[0]);
  assert.eq(fields[fields.length-1], "host",
      "first row should end with added 'host' field");
  clearRawMongoProgramOutput();

  // named
  x = runMongoProgram("mongostat", "--port", port,
      "-o", "host=H,conn=C,time=MYTiME", "-n", 4);
  assert.eq(x, 0, "mongostat should succeed with -o and -n options");
  rows = statRows();
  assert.eq(rows.length, 5, "expected 5 rows in mongostat output");
  assert.eq(statFields(rows[0]).join(), "H,C,MYTiME",
      "first row doesn't match 'H C MYTiME'");
  assert.eq(statFields(rows[1]).length, 3,
      "there should be exactly three entries for a row of this stat output");
  clearRawMongoProgramOutput();

  // serverStatus custom field
  x = runMongoProgram("mongostat", "--port", port,
      "-o", "host,time,mem.bits", "-n", 4);
  assert.eq(x, 0, "mongostat should succeed with -o and -n options");
  rows = statRows();
  assert.eq(rows.length, 5, "expected 5 rows in mongostat output");
  assert.eq(statFields(rows[0]).join(), "host,time,mem.bits",
      "first row doesn't match 'host time mem.bits'");
  fields = statFields(rows[1]);
  assert.eq(fields.length, 3,
      "there should be exactly three entries for a row of this stat output");
  assert(fields[2] === "32" || fields[2] === "64",
      "mem.bits didn't yield valid output (should be one of 32 or 64, was '"
      +fields[2]+"')");
  clearRawMongoProgramOutput();

  // serverStatus named field
  x = runMongoProgram("mongostat", "--port", port,
      "-o", "host,time=MYTiME,mem.bits=BiTs", "-n", 4);
  assert.eq(x, 0, "mongostat should succeed with -o and -n options");
  rows = statRows();
  assert.eq(rows.length, 5, "expected 5 rows in mongostat output");
  assert.eq(statFields(rows[0]).join(), "host,MYTiME,BiTs",
      "first row doesn't match 'host MYTiME BiTs'");
  fields = statFields(rows[1]);
  assert.eq(fields.length, 3,
      "there should be exactly three entries for a row of this stat output");
  assert(fields[2] === "32" || fields[2] === "64",
      "mem.bits didn't yield valid output (should be one of 32 or 64, was '"
      +fields[2]+"')");
  clearRawMongoProgramOutput();

  toolTest.stop();
}());
