// MIGRATION: NEW — no Go test exercises deeply nested BSON documents; goes in bsondump/bsondump_test.go
// This test checks that bsondump can handle a deeply nested document without breaking

(function() {
  var x = _runMongoProgram("bsondump", "--type=json", "jstests/bson/testdata/deep_nested.bson");
  assert.eq(x, 0, "bsondump should exit successfully with 0");
  x = _runMongoProgram("bsondump", "--type=debug", "jstests/bson/testdata/deep_nested.bson");
  assert.eq(x, 0, "bsondump should exit successfully with 0");
}());
