const exitCodeSuccess = 0;
const exitCodeFailure = 1;

// shellRowRegex matches all lines of shell output
const shellRowRegex = /^sh\d+\|\s+/;
// defaultStatLineRegex matches default mongostat stat lines (ends in a timestamp, has at least 16 space delimited fields)
const defaultStatLineRegex = /^sh\d+\|(\s+\S+){16}.*[A-Z][a-z]{2}\s(\s\d|\d\d)\s\d{2}:\d{2}:\d{2}\.\d{3}$/;
// portRegex finds the port on a stat line
const portRegex = /^\S+:(\d+)/;

function isDefaultStatLine(r) {
  return !r.includes("error") && !r.includes("Error") && r.match(defaultStatLineRegex);
}

function filterShellRows(chunk, matcherFunc) {
  return chunk
    .split("\n")
    .filter(r => r.match(shellRowRegex) && matcherFunc(r))
    .map(function(r) {
      return r.replace(/^sh\d+\|\s+/, "");
    });
}

// get all rows of shell output
function allShellRows() {
  return filterShellRows(rawMongoProgramOutput(), () => true);
}

function allDefaultStatRows() {
  return filterShellRows(rawMongoProgramOutput(), isDefaultStatLine);
}

function statFields(row) {
  return row.split(/\s/).filter(function(s) {
    return s !== "";
  });
}

function getLatestChunk() {
  var output = rawMongoProgramOutput();
  // mongostat outputs a blank line between each set of stats when there are
  // multiple hosts; we want just one chunk of stat lines
  var lineChunks = output.split("| \n");
  if (lineChunks.length === 1) {
    return lineChunks[0];
  }
  return lineChunks[lineChunks.length - 2];
}

function getPortFromStatLine(line) {
  const matches = line.match(portRegex);
  if (matches === null) {
    return null;
  }

  return matches[1];
}

function latestPortCounts() {
  var portCounts = {};
  filterShellRows(getLatestChunk(), isDefaultStatLine).forEach(function(r) {
    const port = getPortFromStatLine(r);
    if (port === null) {
      return;
    }

    if (!portCounts[port]) {
      portCounts[port] = 0;
    }
    portCounts[port]++;
  });
  return portCounts;
}

function hasPort(port) {
  port = String(port);
  return function() {
    return latestPortCounts()[port] >= 1;
  };
}

function lacksPort(port) {
  port = String(port);
  return function() {
    return latestPortCounts()[port] === undefined;
  };
}

function hasOnlyPorts(expectedPorts) {
  expectedPorts = expectedPorts.map(String);
  return function() {
    var portCounts = latestPortCounts();
    for (var port in portCounts) {
      if (expectedPorts.indexOf(port) === -1) {
        return false;
      }
    }
    for (var i in expectedPorts) {
      if (portCounts[expectedPorts[i]] !== 1) {
        return false;
      }
    }
    return true;
  };
}

function statCheck(args, checker) {
  clearRawMongoProgramOutput();
  pid = startMongoProgramNoConnect.apply(null, args);
  try {
    assert.soon(checker, "discoverTest wait timed out");
    return true;
  } catch (e) {
    return false;
  } finally {
    stopMongoProgramByPid(pid);
  }
}

function discoverTest(ports, connectHost) {
  return statCheck(["mongostat",
    "--host", connectHost,
    "--noheaders",
    "--discover"],
  hasOnlyPorts(ports));
}

