package mongoplay

type Options struct {
	Verbose      bool `short:"v" long:"verbose" description:"Show verbose debug information"`
	PlaybackFile struct {
		PlaybackFile string `required:"yes" positional-args:"yes" description:"path to the playback file to write to" positional-arg-name:"<playback-file>"`
	} `required:"yes" positional-args:"yes" description:"path to the playback file to write to" positional-arg-name:"<playback-file>"`
}
