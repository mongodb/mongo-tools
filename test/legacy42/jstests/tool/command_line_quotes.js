jsTest.log("Testing spaces in mongodump command-line options...");

/**
 * Starts an instance of the specified mongo tool
 *
 * @param {String} binaryName The name of the tool to run
 * @param {Object} opts options to pass to the tool
 *    {
 *      binVersion {string}: version of tool to run
 *    }
 * @param {String} binaryName - The name of the tool to run.
 * @param {Object} [opts={}] - Options of the form --flag or --key=value to pass to the tool.
 * @param {string} [opts.binVersion] - The version of the tool to run.
 *
 * @param {...string} positionalArgs - Positional arguments to pass to the tool after all
 * options have been specified. For example,
 * MongoRunner.runMongoTool("executable", {key: value}, arg1, arg2) would invoke
 * ./executable --key value arg1 arg2.
 *
 * @see MongoRunner.arrOptions
 */
MongoRunner.runMongoTool = function(binaryName, opts, ...positionalArgs) {

    var opts = opts || {};
    // Normalize and get the binary version to use

    // Convert 'opts' into an array of arguments.
    var argsArray = MongoRunner.arrOptions(binaryName, opts);

    // Append any positional arguments that were specified.
    argsArray.push(...positionalArgs);

    return runMongoProgram.apply(null, argsArray);

};


// var runner = MongoRunner
MongoRunner.dataPath = "./"
MongoRunner.dataDir= "./"
var mongod = MongoRunner.runMongod();
var coll = mongod.getDB("spaces").coll;
coll.drop();
coll.insert({a: 1});
coll.insert({a: 2});

var query = "{\"a\": {\"$gt\": 1} }";
assert(!MongoRunner.runMongoTool(
    "mongodump",
    {"host": "127.0.0.1:" + mongod.port, "db": "spaces", "collection": "coll", "query": query}));

MongoRunner.stopMongod(mongod);

jsTest.log("Test completed successfully");
