var getToolTest;

(function() {
  var AUTH_USER = 'passwordIsTaco';
  var AUTH_PASSWORD = 'Taco';

  var TOOLS_TEST_CONFIG = {
    binVersion: '',
    auth: ''
  };

  getToolTest = function(name) {
    var toolTest = new ToolTest(name, TOOLS_TEST_CONFIG);
    var db = t.startDB();

    db.getSiblingDB('admin').createUser({
      user: AUTH_USER,
      pwd: AUTH_PASSWORD,
      roles: ['__system']
    });

    db.getSiblingDB('admin').auth(AUTH_USER, AUTH_PASSWORD);

    return toolTest;
  };
})();

var getCommonToolArguments = function() {
  return [
    '--username', AUTH_USER,
    '--password', AUTH_PASSWORD
  ];
};
