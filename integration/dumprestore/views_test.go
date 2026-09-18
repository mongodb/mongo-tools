package dumprestore

import (
	"maps"
	"slices"

	"github.com/mongodb/mongo-tools/mongorestore"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const citiesCollName = "cities"

// viewCities is used as data for the view tests. For each US state, we have a different number of
// cities. This ensures that if a view's pipeline is restored with the wrong `$match`, it cannot
// match the right count by accident.
var viewCities = map[string][]string{
	"ID": {"Boise", "Nampa", "Pocatello"},
	"NY": {"Albany", "New York"},
	"CA": {"Los Angeles", "San Jose", "Cupertino", "San Francisco"},
}

// TestViewsRoundTrip covers what happens to read-only views across a dump and restore, in both of
// the modes mongodump offers for them.
func (s *DumpRestoreSuite) TestViewsRoundTrip() {
	s.Run("views are restored as views", s.testViewsRestoredAsViews)
	s.Run("restoring a second time over the data", s.testViewsRestoredTwice)
	s.Run("viewsAsCollections restores views as collections", s.testViewsAsCollections)
}

// testViewsRestoredAsViews checks that a default dump and restore brings back the backing
// collection and the views over it, and that what comes back is still a view rather than a
// collection holding a snapshot of what the view matched.
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
	s.assertAreViews(testDB, slices.Collect(maps.Keys(viewCities))...)
}

// testViewsRestoredTwice restores a second time with --drop over the data the first restore
// produced. A view cannot be dropped and recreated the way a collection can, so this is the case
// where a restore that treated views as ordinary collections would fail or leave the view behind as
// a collection.
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

		// Without --stopOnError a restore whose --drop did not take effect reports no error, just a
		// duplicate key failure per document. The city assertions below would still pass on what
		// the first restore left behind, so the insert counts are what distinguishes a real second
		// restore from that.
		s.assertRestoredEveryCity(second)
	}, "--db", testDB.Name())

	s.assertCitiesRestored(testDB)
	s.assertViewsMatchTheirState(testDB)
	s.assertAreViews(testDB, slices.Collect(maps.Keys(viewCities))...)
}

// testViewsAsCollections checks --viewsAsCollections, which dumps each view as an ordinary
// collection holding the documents it matched and omits the real collections entirely. The restored
// views therefore have to come back as collections, and the backing collection has to be absent.
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
	s.assertAreCollections(testDB, slices.Collect(maps.Keys(viewCities))...)
	s.Assert().NotContains(
		s.collectionNames(testDB),
		citiesCollName,
		"the collection the views were built on is absent, because --viewsAsCollections did not dump it",
	)
}

func (s *DumpRestoreSuite) createCitiesAndViews(testDB *mongo.Database) {
	_, err := testDB.Collection(citiesCollName).InsertMany(s.Context(), s.viewCitiesDocs())
	s.Require().NoError(err, "can insert the cities")

	for state := range viewCities {
		err := testDB.CreateView(
			s.Context(),
			state,
			citiesCollName,
			mongo.Pipeline{bson.D{{"$match", bson.D{{"state", state}}}}},
		)
		s.Require().NoError(err, "can create the view for state %#q", state)
	}
}

// assertRestoredEveryCity checks the restore inserted one document per city. That count holds in
// both dump modes: a default dump carries the cities once in the backing collection and the views
// carry no data of their own, while --viewsAsCollections carries each city once in the view it
// belongs to.
func (s *DumpRestoreSuite) assertRestoredEveryCity(result mongorestore.Result) {
	s.Assert().EqualValues(len(s.viewCitiesDocs()), result.Successes, "every city is inserted")
	s.Assert().EqualValues(0, result.Failures, "no document is rejected")
}

func (s *DumpRestoreSuite) assertCitiesRestored(testDB *mongo.Database) {
	s.Assert().ElementsMatch(
		lo.Flatten(slices.Collect(maps.Values(viewCities))),
		s.documentIDs(testDB.Collection(citiesCollName)),
		"every city is restored",
	)
}

func (_ *DumpRestoreSuite) viewCitiesDocs() []bson.D {
	var docs []bson.D
	for state, cities := range viewCities {
		for _, c := range cities {
			docs = append(docs, bson.D{{"_id", c}, {"state", state}})
		}
	}
	return docs
}

// assertViewsMatchTheirState checks the contents of each state's view rather than only its document
// count, so that a view restored with a pipeline matching the wrong state cannot pass by matching
// the right number of cities.
func (s *DumpRestoreSuite) assertViewsMatchTheirState(testDB *mongo.Database) {
	for state := range viewCities {
		s.Assert().ElementsMatch(
			viewCities[state],
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
