(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }
  var toolTest = getToolTest('views');
  var db = toolTest.db;
  var commonToolArgs = getCommonToolArguments();

  var dumpTarget = 'views_dump';
  resetDbpath(dumpTarget);

  var supportsViews = typeof db.createView === 'function';

  function addCitiesData() {
    db.cities.insertMany([{
      city: 'Boise',
      state: 'ID',
    }, {
      city: 'Pocatello',
      state: 'ID',
    }, {
      city: 'Nampa',
      state: 'ID',
    }, {
      city: 'Albany',
      state: 'NY',
    }, {
      city: 'New York',
      state: 'NY',
    }, {
      city: 'Los Angeles',
      state: 'CA',
    }, {
      city: 'San Jose',
      state: 'CA',
    }, {
      city: 'Cupertino',
      state: 'CA',
    }, {
      city: 'San Francisco',
      state: 'CA',
    }]);
  }

  function addStateView(state) {
    db.createCollection('cities'+state, {
      viewOn: 'cities',
      pipeline: [{
        $match: {state: state},
      }],
    });
  }

  addCitiesData();
  addStateView('ID');
  addStateView('NY');
  addStateView('CA');

  assert.eq(9, db.cities.count(), 'should have 9 cities');
  if (supportsViews) {
    assert.eq(3, db.citiesID.count(), 'should have 3 cities in Idaho view');
    assert.eq(2, db.citiesNY.count(), 'should have 2 cities in New York view');
    assert.eq(4, db.citiesCA.count(), 'should have 4 cities in California view');
  }

  var ret;

  ret = toolTest.runTool.apply(toolTest, ['dump']
    .concat(getDumpTarget(dumpTarget))
    .concat(commonToolArgs));
  assert.eq(0, ret, 'dump should succeed');

  db.dropDatabase();

  ret = toolTest.runTool.apply(toolTest, ['restore']
    .concat(getRestoreTarget(dumpTarget))
    .concat(commonToolArgs));
  assert.eq(0, ret, 'restore should succeed');

  assert.eq(9, db.cities.count(), 'should have 9 cities');
  if (supportsViews) {
    assert.eq(3, db.citiesID.count(), 'should have 3 cities in Idaho view');
    assert.eq(2, db.citiesNY.count(), 'should have 2 cities in New York view');
    assert.eq(4, db.citiesCA.count(), 'should have 4 cities in California view');
  }

  ret = toolTest.runTool.apply(toolTest, ['restore', '--drop']
    .concat(getRestoreTarget(dumpTarget))
    .concat(commonToolArgs));
  assert.eq(0, ret, 'restore --drop should succeed');

  assert.eq(9, db.cities.count(), 'should have 9 cities');
  if (supportsViews) {
    assert.eq(3, db.citiesID.count(), 'should have 3 cities in Idaho view');
    assert.eq(2, db.citiesNY.count(), 'should have 2 cities in New York view');
    assert.eq(4, db.citiesCA.count(), 'should have 4 cities in California view');
  }

  resetDbpath(dumpTarget);

  ret = toolTest.runTool.apply(toolTest, ['dump', '--viewsAsCollections']
    .concat(getDumpTarget(dumpTarget))
    .concat(commonToolArgs));
  assert.eq(0, ret, 'dump --viewsAsCollections should succeed');

  db.dropDatabase();

  ret = toolTest.runTool.apply(toolTest, ['restore']
    .concat(getRestoreTarget(dumpTarget))
    .concat(commonToolArgs));
  assert.eq(0, ret, 'restore should succeed');

  assert.eq(0, db.cities.count(), 'should not have cities collection');
  if (supportsViews) {
    assert.eq(3, db.citiesID.count(), 'should have 3 cities in Idaho collections');
    assert.eq(2, db.citiesNY.count(), 'should have 2 cities in New York collections');
    assert.eq(4, db.citiesCA.count(), 'should have 4 cities in California collections');
  }

  resetDbpath(dumpTarget);
  toolTest.stop();
}());
