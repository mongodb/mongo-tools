var exitCodeSuccess = 0;
var exitCodeErr = 1;
// Go reserves exit code 2 for its own use.
var exitCodeBadOptions = 3;
var exitCodeStopped = 4;

var rowRegex = /^sh\d+\|\s/;
// portRegex finds the port on a line which has enough whitespace-delimited
// values to be considered a stat line and not an error message
var portRegex = /^sh\d+\|\s+\S+:(\d+)(\s+\S+){16}/;

function discoverTest(ports, connectHost) {
    clearRawMongoProgramOutput();
    x = runMongoProgram.apply(null, [ "mongostat", "--host", connectHost, "--rowcount", 7, "--noheaders", "--discover" ]);
    return statOutputPortCheck(ports);
}

function statOutputPortCheck(ports) {
    var portMap = {};
    ports.forEach(function(p) {
        portMap[p] = true;
    });
    var output = rawMongoProgramOutput();
    // mongostat outputs a blank line between each set of stats when there are
    // multiple hosts; we want just one chunk of stat lines
    var lineChunks = output.split("| \n");
    var checkDupes = false;
    var foundChunk = lineChunks[0];
    if (lineChunks.length > 1) {
        checkDupes = true;
        // With multiple hosts, use only the last complete chunk of stat lines
        // We assume that being bounded by blank lines implies it is complete
        foundChunk = lineChunks[lineChunks.length - 2];
    }
    var foundRows = foundChunk.split("\n").filter(function(r) {
        return r.match(portRegex);
    });
    var foundPorts = foundRows.map(function(r) {
        return r.match(portRegex)[1];
    });
    foundPorts.forEach(function(p) {
        portMap[p] = false;
    });
    var somePortsUnseen = ports.some(function(p) {
        return portMap[p];
    });
    var noDupes = foundPorts.every(function(p, i) {
        return foundPorts.indexOf(p) === i;
    });
    return (!checkDupes || noDupes) && !somePortsUnseen;
}

