// Wrap whole file in a function to avoid polluting the global namespace
(function () {
  let oldAssertAuthenticate = authutil.assertAuthenticate;
  authutil.assertAuthenticate = function(conns, dbName, authParams) {
    print("authutil.assertAuthenticate overriden in mongo-tools");

    if (authParams !== undefined && authParams.user === "__system") {
      authParams.mechanism = 'SCRAM-SHA-256';
    }

    return oldAssertAuthenticate(conns, dbName, authParams);
  };
}());
