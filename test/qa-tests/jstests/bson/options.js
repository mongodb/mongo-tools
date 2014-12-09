// This test checks reasonable and unreasonable option configurations for bsondump

(function(){
    var sampleFilepath = "jstests/bson/testdata/sample.bson"
    var x = runMongoProgram( "bsondump", "--type=fake", sampleFilepath);
    assert.neq(x, 0, "bsondump should exit with failure when given a non-existent type");

    x = runMongoProgram( "bsondump", "jstests/bson/testdata/asdfasdfasdf");
    assert.neq(x, 0, "bsondump should exit with failure when given a non-existent file");

    x = runMongoProgram( "bsondump", "--noobjcheck", sampleFilepath);
    assert.neq(x, 0, "bsondump should exit with failure when given --noobjcheck");

    x = runMongoProgram( "bsondump", "--collection", sampleFilepath);
    assert.neq(x, 0, "bsondump should exit with failure when given --collection");

    x = runMongoProgram( "bsondump", sampleFilepath, sampleFilepath);
    assert.neq(x, 0, "bsondump should exit with failure when given multiple files");
   
    x = runMongoProgram( "bsondump", "-vvvv", sampleFilepath);
    assert.eq(x, 0, "bsondump should exit with success when given verbosity");
    x = runMongoProgram( "bsondump", "--verbose", sampleFilepath);
    assert.eq(x, 0, "bsondump should exit with success when given verbosity");

    clearRawMongoProgramOutput()
    x = runMongoProgram( "bsondump", "--quiet", sampleFilepath);
    assert.eq(x, 0, "bsondump should exit with success when given --quiet");
    var results = rawMongoProgramOutput();
    assert.eq(results.search("found"), -1, "only the found docs should be printed when --quiet is used");
    assert.gt(results.search("I am a string"), -1, "found docs should still be printed when --quiet is used");

    clearRawMongoProgramOutput()
    x = runMongoProgram( "bsondump", "--help");
    assert.eq(x, 0, "bsondump should exit with success when given --help");
    var results = rawMongoProgramOutput();
    assert.gt(results.search("Usage"), -1, "help text should be printed when given --help");

    clearRawMongoProgramOutput()
    x = runMongoProgram( "bsondump", "--version");
    assert.eq(x, 0, "bsondump should exit with success when given --version");
    var results = rawMongoProgramOutput();
    assert.gt(results.search("version"), -1, "version info should be printed when given --version");

})();
