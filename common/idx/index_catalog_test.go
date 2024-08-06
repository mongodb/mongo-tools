package idx

import (
	"testing"

	"github.com/mongodb/mongo-tools/common/testtype"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestDeleteIndexes(t *testing.T) {
	require := require.New(t)

	testtype.SkipUnlessTestType(t, testtype.UnitTestType)

	t.Run("drop one index by name", func(t *testing.T) {
		i := newTestIndexCatalog(t)

		dropFooBarField1Cmd := bson.D{{"dropIndexes", "foo"}, {"index", "foo_bar_field1_idx"}}
		require.NoError(i.DeleteIndexes("foo", "bar", dropFooBarField1Cmd))
		require.Nil(
			i.GetIndex("foo", "bar", "foo_bar_field1_idx"),
			"dropped foo_bar_field1_idx index",
		)
		require.NotNil(i.GetIndex("foo", "bar", "_id_"), "bar._id_ index still exists")
		require.NotNil(i.GetIndex("foo", "baz", "foo_baz_idx"), "foo_baz_idx index still exists")
		require.NotNil(
			i.GetIndex("foo", "baz", "_id_clustered_index"),
			"_id_clustered_index index still exists",
		)
	})

	t.Run("drop one index by definition", func(t *testing.T) {
		i := newTestIndexCatalog(t)

		dropFooBarField1Cmd := bson.D{{"dropIndexes", "foo"}, {"index", bson.D{{"field1", 1}}}}
		require.NoError(i.DeleteIndexes("foo", "bar", dropFooBarField1Cmd))
		require.Nil(
			i.GetIndex("foo", "bar", "foo_bar_field1_idx"),
			"dropped foo_bar_field1_idx index",
		)
		require.NotNil(i.GetIndex("foo", "bar", "_id_"), "bar._id_ index still exists")
		require.NotNil(i.GetIndex("foo", "baz", "foo_baz_idx"), "foo_baz_idx index still exists")
		require.NotNil(
			i.GetIndex("foo", "baz", "_id_clustered_index"),
			"_id_clustered_index index still exists",
		)
	})

	t.Run("drop all indexes", func(t *testing.T) {
		i := newTestIndexCatalog(t)

		dropStarCmd := bson.D{{"dropIndexes", "foo"}, {"index", "*"}}
		require.NoError(i.DeleteIndexes("foo", "bar", dropStarCmd))
		require.Nil(
			i.GetIndex("foo", "bar", "foo_bar_field1_idx"),
			"dropped foo_bar_field1_idx index",
		)
		require.NotNil(i.GetIndex("foo", "bar", "_id_"), "bar._id_ index still exists")
		require.NotNil(i.GetIndex("foo", "baz", "foo_baz_idx"), "foo_baz_idx index still exists")
		require.NotNil(
			i.GetIndex("foo", "baz", "_id_clustered_index"),
			"_id_clustered_index index still exists",
		)

		require.NoError(i.DeleteIndexes("foo", "baz", dropStarCmd))
		require.Nil(
			i.GetIndex("foo", "bar", "foo_bar_field1_idx"),
			"dropped foo_bar_field1_idx index",
		)
		require.NotNil(i.GetIndex("foo", "bar", "_id_"), "bar._id_ index still exists")
		require.Nil(i.GetIndex("foo", "baz", "foo_baz_idx"), "dropped foo_baz_idx index")
		require.NotNil(
			i.GetIndex("foo", "baz", "_id_clustered_index"),
			"_id_clustered_index index still exists",
		)
	})
}

func newTestIndexCatalog(t *testing.T) *IndexCatalog {
	require := require.New(t)

	i := NewIndexCatalog()

	fooBarIdDoc, err := NewIndexDocumentFromD(bson.D{{"key", bson.D{{"_id", 1}}}})
	require.NoError(err, "no error creating index document for foo.bar._id index")

	fooBarField1Doc, err := NewIndexDocumentFromD(bson.D{{"key", bson.D{{"field1", 1}}}})
	require.NoError(err, "no error creating index document for foo.bar.field1 index")

	fooBazIdDoc, err := NewIndexDocumentFromD(bson.D{{"key", bson.D{{"_id", 1}}}})
	require.NoError(err, "no error creating index document for foo.baz._id index")

	fooBazField2Doc, err := NewIndexDocumentFromD(bson.D{{"key", bson.D{{"field2", 1}}}})
	require.NoError(err, "no error creating index document for foo.baz.field2 index")

	i.addIndex("foo", "bar", "_id_", fooBarIdDoc)
	i.addIndex("foo", "bar", "foo_bar_field1_idx", fooBarField1Doc)
	i.addIndex("foo", "baz", "_id_clustered_index", fooBazIdDoc)
	i.addIndex("foo", "baz", "foo_baz_idx", fooBazField2Doc)
	require.NotNil(i.GetIndex("foo", "bar", "foo_bar_field1_idx"))
	require.NotNil(i.GetIndex("foo", "baz", "foo_baz_idx"))

	return i
}
