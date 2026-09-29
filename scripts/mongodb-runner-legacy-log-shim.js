"use strict";

// Temporary workaround for mongodb-runner hanging against MongoDB 4.2.
//
// mongodb-runner starts mongod with `--port 0` and then reads the chosen port from the server's log,
// but it only recognizes the structured (JSON) "Waiting for connections" entry (id 23016). MongoDB
// 4.2 predates structured logging, so the port is never found and `mongodb-runner start` waits
// forever. This shim patches the runner's log reader at load time to also read the legacy text line.
// It is a no-op for 4.4+, and does nothing if the runner's source no longer matches. Load it with
// NODE_OPTIONS=--require=<this file>. Remove it once mongodb-runner handles legacy logs itself.

const Module = require("module");

const originalCompile = Module.prototype._compile;

Module.prototype._compile = function (content, filename) {
  if (
    filename.endsWith("mongologreader.js") &&
    filename.includes("mongodb-runner")
  ) {
    content = content.replace(
      "if (logEntry.id === 23016) {",
      "if (logEntry.id === 23016) {\n" +
        "        return logEntry.attr.port;\n" +
        "    }\n" +
        '    if (logEntry.id === undefined && typeof logEntry.message === "string") {\n' +
        "        const legacyPort = /waiting for connections on port (\\d+)/.exec(logEntry.message);\n" +
        "        if (legacyPort) { return Number(legacyPort[1]); }\n" +
        "    }\n" +
        "    if (false) {",
    );
  }

  return originalCompile.call(this, content, filename);
};
