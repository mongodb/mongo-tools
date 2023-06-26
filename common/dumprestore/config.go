package dumprestore

// ConfigCollectionsToKeep defines a list of collections in the `config`
// database that we include by default in backups and restores.
//
// These are the only config collections that are dumped when dumping
// and entire cluster. If you set mognodump --db=config then everything
// in the config collection is included.
//
// These are the only collections that mongorestore will apply oplog events for
// if --replayOplog is set.
var ConfigCollectionsToKeep = []string{
	"chunks",
	"collections",
	"databases",
	"settings",
	"shards",
	"tags",
	"version",
}
