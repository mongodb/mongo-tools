var getDumpTarget;

(function() {
  if (getDumpTarget === undefined) {
    getDumpTarget = function(target) {
      if (target === undefined) {
        return [];
      }
      return ["--out="+target];
    };
  }
}());

var dump_targets;
if (!dump_targets) {
  dump_targets = "standard";
}

var getRestoreTarget;

(function() {
  if (getRestoreTarget === undefined) {
    getRestoreTarget = function(target) {
      if (target === undefined) {
        return [];
      }
      return ["--dir="+target];
    };
  }
}());

/* exported getCommonToolArguments */
var getCommonToolArguments = function() {
  return ['--ssl',
    '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem',
    '--sslAllowInvalidHostnames'];
};

