// This test runs bsondump on a .bson file containing non-deprecated BSON types
// and makes sure their JSON type representations exist in the output.
(function() {
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;
  load("jstests/libs/output.js");

  const doc = {
    "double": 2.0,
    "string": "hi",
    "doc": {"x": 1},
    "arr": [1, 2],
    "binary": {"$binary": {"base64": "//8=", "subType": "00"}},
    "oid": {"$oid": "507f1f77bcf86cd799439011"},
    "bool": true,
    "date": {"$date": "2001-01-01T01:23:20Z"},
    "code": {"$code": "hi", "$scope": {"x": 1}},
    "ts": {"$timestamp": {"t": 1, "i": 2}},
    "int32": 5,
    "int64": 6,
    "dec": {"$numberDecimal": "1.2E+10"},
    "minkey": {"$minKey": 1},
    "maxkey": {"$maxKey": 1},
    "regex": {"$regularExpression": {"pattern": "^abc", "options": "imx"}},
    "symbol": {"$symbol": "i am a symbol"},
    "undefined": {"$undefined": true},
    "dbpointer": {"$dbPointer": {"$ref": "some.namespace", "$id": {"$oid": "507f1f77bcf86cd799439011"}}},
    "null": null
  };

  const x = _runMongoProgram("bsondump", "--type=json", "jstests/bson/testdata/all_in_one_doc.bson");
  assert.eq(x, 0, "bsondump should exit successfully with 0");

  assert.strContains.soon("1 objects found", rawMongoProgramOutput,
    "should print out all top-level documents from the test data");

  // get row of output containing the json
  const results = filterShellRows(rawMongoProgramOutput(), row => row.indexOf("$oid") !== -1);
  assert.eq(results.length, 1);
  assert.eq(JSON.parse(results[0]), doc);
}());
