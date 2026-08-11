package dumprestore

import (
	"github.com/mongodb/mongo-tools/mongorestore"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const citiesCollName = "cities"

type cityFixture struct {
	city  string
	state string
}

// viewCities is the fixture: one city per document, with a view per state. The
// states hold different numbers of cities so that a view whose pipeline was
// restored with the wrong $match cannot match the right count by accident.
var viewCities = []cityFixture{
	{"Boise", "ID"},
	{"Pocatello", "ID"},
	{"Nampa", "ID"},
	{"Albany", "NY"},
	{"New York", "NY"},
	{"Los Angeles", "CA"},
	{"San Jose", "CA"},
	{"Cupertino", "CA"},
	{"San Francisco", "CA"},
}

var viewStates = []string{"ID", "NY", "CA"}

// TestViewsRoundTrip covers what happens to read-only views across a dump and
// restore, in both of the modes mongodump offers for them.
func (s *DumpRestoreSuite) TestViewsRoundTrip() {
	s.Run("views are restored as views", s.testViewsRestoredAsViews)
	s.Run("restoring a second time over the data", s.testViewsRestoredTwice)
	s.Run("viewsAsCollections restores views as collections", s.testViewsAsCollections)
}

// testViewsRestoredAsViews checks that a default dump and restore brings back the
// backing collection and the views over it, and that what comes back is still a
// view rather than a collection holding a snapshot of what the view matched.
func (s *DumpRestoreSuite) testViewsRestoredAsViews() {
	testDB := s.database("views")
	s.createCitiesAndViews(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore a dump holding views")
		s.assertRestoredEveryCity(result)
	}, "--db", testDB.Name())

	s.assertCitiesRestored(testDB)
	s.assertViewsMatchTheirState(testDB)
	s.assertAreViews(testDB, viewStates...)
}

// testViewsRestoredTwice restores a second time with --drop over the data the
// first restore produced. A view cannot be dropped and recreated the way a
// collection can, so this is the case where a restore that treated views as
// ordinary collections would fail or leave the view behind as a collection.
func (s *DumpRestoreSuite) testViewsRestoredTwice() {
	testDB := s.database("views_twice")
	s.createCitiesAndViews(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		first := s.runRestore(dir)
		s.Require().NoError(first.Err, "can restore a dump holding views")
		s.assertRestoredEveryCity(first)

		second := s.runRestore(mongorestore.DropOption, dir)
		s.Require().NoError(second.Err, "can restore the same dump again with --drop")

		// Without --stopOnError a restore whose --drop did not take effect reports
		// no error, just a duplicate key failure per document. The city assertions
		// below would still pass on what the first restore left behind, so the
		// insert counts are what distinguishes a real second restore from that.
		s.assertRestoredEveryCity(second)
	}, "--db", testDB.Name())

	s.assertCitiesRestored(testDB)
	s.assertViewsMatchTheirState(testDB)
	s.assertAreViews(testDB, viewStates...)
}

// testViewsAsCollections checks --viewsAsCollections, which dumps each view as an
// ordinary collection holding the documents it matched and omits the real
// collections entirely. The restored views therefore have to come back as
// collections, and the backing collection has to be absent.
func (s *DumpRestoreSuite) testViewsAsCollections() {
	testDB := s.database("views_as_collections")
	s.createCitiesAndViews(testDB)

	s.withBSONMongodump(func(dir string) {
		s.dropDB(testDB)

		result := s.runRestore(dir)
		s.Require().NoError(result.Err, "can restore a --viewsAsCollections dump")
		s.assertRestoredEveryCity(result)
	}, "--db", testDB.Name(), "--viewsAsCollections")

	s.assertViewsMatchTheirState(testDB)
	s.assertAreCollections(testDB, viewStates...)
	s.Assert().NotContains(
		s.collectionNames(testDB),
		citiesCollName,
		"the collection the views were built on is absent, because --viewsAsCollections did not dump it",
	)
}

func (s *DumpRestoreSuite) createCitiesAndViews(testDB *mongo.Database) {
	docs := lo.Map(viewCities, func(city cityFixture, _ int) any {
		return bson.D{{"_id", city.city}, {"state", city.state}}
	})
	_, err := testDB.Collection(citiesCollName).InsertMany(s.Context(), docs)
	s.Require().NoError(err, "can insert the fixture cities")

	for _, state := range viewStates {
		err := testDB.CreateView(
			s.Context(),
			state,
			citiesCollName,
			mongo.Pipeline{bson.D{{"$match", bson.D{{"state", state}}}}},
		)
		s.Require().NoError(err, "can create the view for state %#q", state)
	}
}

// assertRestoredEveryCity checks the restore inserted one document per fixture
// city. That count holds in both dump modes: a default dump carries the cities
// once in the backing collection and the views carry no data of their own, while
// --viewsAsCollections carries each city once in the view it belongs to.
func (s *DumpRestoreSuite) assertRestoredEveryCity(result mongorestore.Result) {
	s.Assert().EqualValues(len(viewCities), result.Successes, "every city is inserted")
	s.Assert().EqualValues(0, result.Failures, "no document is rejected")
}

func (s *DumpRestoreSuite) assertCitiesRestored(testDB *mongo.Database) {
	s.Assert().ElementsMatch(
		lo.Map(viewCities, func(city cityFixture, _ int) string {
			return city.city
		}),
		s.documentIDs(testDB.Collection(citiesCollName)),
		"every fixture city is restored",
	)
}

// assertViewsMatchTheirState checks the contents of each state's view rather than
// only its document count, so that a view restored with a pipeline matching the
// wrong state cannot pass by matching the right number of cities.
func (s *DumpRestoreSuite) assertViewsMatchTheirState(testDB *mongo.Database) {
	for _, state := range viewStates {
		wantCities := lo.FilterMap(viewCities, func(city cityFixture, _ int) (string, bool) {
			return city.city, city.state == state
		})

		s.Assert().ElementsMatch(
			wantCities,
			s.documentIDs(testDB.Collection(state)),
			"the view for state %#q holds that state's cities",
			state,
		)
	}
}

func (s *DumpRestoreSuite) assertAreViews(testDB *mongo.Database, names ...string) {
	for _, name := range names {
		s.Assert().Equal(
			"view",
			s.collectionType(testDB, name),
			"%#q comes back as a view, not as a collection of what it matched",
			name,
		)
	}
}

func (s *DumpRestoreSuite) assertAreCollections(testDB *mongo.Database, names ...string) {
	for _, name := range names {
		s.Assert().Equal(
			"collection",
			s.collectionType(testDB, name),
			"%#q comes back as an ordinary collection",
			name,
		)
	}
}

func (s *DumpRestoreSuite) collectionType(testDB *mongo.Database, name string) string {
	specs, err := testDB.ListCollectionSpecifications(
		s.Context(),
		bson.D{{"name", name}},
	)
	s.Require().NoError(err, "can list the specification of %#q", name)
	s.Require().Len(specs, 1, "%#q exists in %#q", name, testDB.Name())

	return specs[0].Type
}
