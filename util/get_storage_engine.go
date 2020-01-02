package util

import "context"

// WiredTiger represents a  wired tiger storage engine.
const WiredTiger = "wiredTiger"

// MMAPv1 represents a mmapv1 stoage engine.
const MMAPv1 = "MMAPv1"

// GetStorageEngine returns the type of storage engine, either WiredTiger or MMAPv1.
func GetStorageEngine(database *mongo.Database, collectionName string) (string, error) {
	if database == nil {
		return "", errors.New("Invalid input in getStorageEngine")
	}

	var collStats map[string]interface{}

	singleRes := database.RunCommand(context.Background(), bson.M{"collStats": collectionName})
	var storageEngine string

	if err := singleRes.Err(); err == nil {
		if err = singleRes.Decode(&collStats); err != nil {
			return "", errors.Wrap(err, "running parse collStats response")
		}

		if _, ok := collStats[WIRED_TIGER]; ok {
			storageEngine = WIRED_TIGER
		} else {
			storageEngine = MMAPV1
		}
	} else {
		// collStats command failed
		return "", errors.New("Failed to retrieve storage engine name")
	}

	return storageEngine, nil
}
