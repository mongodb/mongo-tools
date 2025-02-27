package db

import (
	"context"

	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const IDLUnknownFieldErrCode = 40415

func MongoCanAcceptLiteralZeroTimestamp(
	ctx context.Context,
	client *mongo.Client,
) (bool, error) {
	coll := client.
		Database("_mongorestore_dummy").
		Collection("_mongorestore_dummy.dummy")

	_, err := coll.UpdateOne(
		ctx,
		bson.D{{"____dummy", 1}},
		bson.D{
			{"$rename", bson.D{}},
		},
		&options.UpdateOptions{
			BypassEmptyTsReplacement: lo.ToPtr(true),
		},
	)

	if err == nil {
		return true, nil
	}

	srvErr, isSrvErr := err.(mongo.ServerError)
	if isSrvErr && srvErr.HasErrorCode(IDLUnknownFieldErrCode) {
		return false, nil
	}

	return false, err
}
