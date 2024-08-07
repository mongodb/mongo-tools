/*
 * Helper functions which connect to a server, and check its logs for particular strings.
 */
var checkLog;

(function() {
  "use strict";

  if (checkLog) {
    return; // Protect against this file being double-loaded.
  }

  checkLog = (function() {
    let getGlobalLog = function(conn) {
      let cmdRes;
      try {
        cmdRes = conn.adminCommand({getLog: 'global'});
      } catch (e) {
        // Retry with network errors.
        print("checkLog ignoring failure: " + e);
        return null;
      }

      return assert.commandWorked(cmdRes).log;
    };

    /*
     * Calls the 'getLog' function on the provided connection 'conn' to see if the provided msg
     * is found in the logs. Note: this function does not throw an exception, so the return
     * value should not be ignored.
     */
    const checkContainsOnce = function(conn, msg) {
      const logMessages = getGlobalLog(conn);
      if (logMessages === null) {
        return false;
      }
      for (let logMsg of logMessages) {
        if (logMsg.includes(msg)) {
          return true;
        }
      }
      return false;
    };

    /*
     * Calls the 'getLog' function at regular intervals on the provided connection 'conn' until
     * the provided 'msg' is found in the logs, or it times out. Throws an exception on timeout.
     */
    let contains = function(conn, msg, timeout = 5 * 60 * 1000) {
      assert.soon(function() {
        return checkContainsOnce(conn, msg);
      }, 'Could not find log entries containing the following message: ' + msg, timeout, 300);
    };

    /*
     * Calls the 'getLog' function at regular intervals on the provided connection 'conn' until
     * the provided 'msg' is found in the logs 'expectedCount' times, or it times out.
     * Throws an exception on timeout. If 'exact' is true, checks whether the count is exactly
     * equal to 'expectedCount'. Otherwise, checks whether the count is at least equal to
     * 'expectedCount'. Early returns when at least 'expectedCount' entries are found.
     */
    let containsWithCount = function(
      conn, msg, expectedCount, timeout = 5 * 60 * 1000, exact = true) {
      let expectedStr = exact ? 'exactly ' : 'at least ';
      assert.soon(
        function() {
          let count = 0;
          let logMessages = getGlobalLog(conn);
          if (logMessages === null) {
            return false;
          }
          for (let i = 0; i < logMessages.length; i++) {
            if (logMessages[i].indexOf(msg) !== -1) {
              count++;
            }
            if (!exact && count >= expectedCount) {
              print("checkLog found at least " + expectedCount +
                    " log entries containing the following message: " + msg);
              return true;
            }
          }

          return exact ? expectedCount === count : expectedCount <= count;
        },
        'Did not find ' + expectedStr + expectedCount + ' log entries containing the ' +
          'following message: ' + msg,
        timeout,
        300);
    };

    /*
     * Similar to containsWithCount, but checks whether there are at least 'expectedCount'
     * instances of 'msg' in the logs.
     */
    let containsWithAtLeastCount = function(conn, msg, expectedCount, timeout = 5 * 60 * 1000) {
      containsWithCount(conn, msg, expectedCount, timeout, /* exact */ false);
    };

    /*
     * Converts a scalar or object to a string format suitable for matching against log output.
     * Field names are not quoted, and by default strings which are not within an enclosing
     * object are not escaped. Similarly, integer values without an enclosing object are
     * serialized as integers, while those within an object are serialized as floats to one
     * decimal point. NumberLongs are unwrapped prior to serialization.
     */
    const formatAsLogLine = function(value, escapeStrings, toDecimal) {
      if (typeof value === "string") {
        return (escapeStrings ? `"${value}"` : value);
      } else if (typeof value === "number") {
        return (Number.isInteger(value) && toDecimal ? value.toFixed(1) : value);
      } else if (value instanceof NumberLong) {
        return `${value}`.match(/NumberLong..(.*)../m)[1];
      } else if (typeof value !== "object") {
        return value;
      } else if (Object.keys(value).length === 0) {
        return Array.isArray(value) ? "[]" : "{}";
      }
      let serialized = [];
      escapeStrings = toDecimal = true;
      for (let fieldName in value) {
        if (fieldName !== "") {
          const valueStr = formatAsLogLine(value[fieldName], escapeStrings, toDecimal);
          serialized.push(Array.isArray(value) ? valueStr : `${fieldName}: ${valueStr}`);
        }
      }
      return (Array.isArray(value) ? `[ ${serialized.join(', ')} ]`
        : `{ ${serialized.join(', ')} }`);
    };

    return {
      getGlobalLog: getGlobalLog,
      checkContainsOnce: checkContainsOnce,
      contains: contains,
      containsWithCount: containsWithCount,
      containsWithAtLeastCount: containsWithAtLeastCount,
      formatAsLogLine: formatAsLogLine
    };
  }());
}());
