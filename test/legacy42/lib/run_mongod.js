/**
 * This file has to be loaded to mongo shell before loading js tests.
 *
 * This file overrides MongoRunner.runMongod function in mongo shell:
 *  - Removes "journal" option before starting mongod to make the deprecated mongo shell compatible with MongoDB 6.1+
 *
 * MongoRunner.runMongod Starts a mongod instance.
 *
 * It also points MongoRunner at the working directory, because these tests run
 * where there is no /data/db.
 */

// Every jstests/tool file is loaded into one shell process, so these are global
// for the whole run. They used to be set as a side effect of
// command_line_quotes.js, which sorts first in the jstests/tool glob and so ran
// before everything else; each later test depended on that without saying so.
// That test was converted to Go in TOOLS-4263, so these live here now,
// alongside the rest of the harness setup.
MongoRunner.dataPath = "./";
MongoRunner.dataDir = "./";

// Starts one of the tools by name (e.g. "mongodump"), converting an options
// object into "--flag" and "--key=value" argv entries plus any positional
// arguments. The shell's own MongoRunner provides arrOptions.
MongoRunner.runMongoTool = function (binaryName, opts, ...positionalArgs) {
    opts = opts || {};
    var argsArray = MongoRunner.arrOptions(binaryName, opts);
    argsArray.push(...positionalArgs);
    return runMongoProgram.apply(null, argsArray);
};

(function () {
    let oldRunMongod = MongoRunner.runMongod;

    MongoRunner.runMongod = function (opts) {
        print("MongoRunner.runMongod overridden in mongo-tools");

        if (opts != undefined && opts.journal != undefined) {
            delete opts.journal;
        }

        return oldRunMongod(opts);
    };
})()
