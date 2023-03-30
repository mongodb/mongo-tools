package dumprestore

// ConfigCollectionsToKeep defines a list of collections in the `config`
// database that we include in backups and restores.
var ConfigCollectionsToKeep = []string{
	"chunks",
	"collections",
	"databases",
	"settings",
	"shards",
	"tags",
	"version",
}
