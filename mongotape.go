package mongotape

type Options struct {
	Verbosity []bool `short:"v" long:"verbosity" description:"increase the detail regarding the tools performance on the input file that is output to logs (include multiple times for increased logging verbosity, e.g. -vvv)"`
	Debug     []bool `short:"d" long:"debug" description:"increase the detail regarding the operations and errors of the tool that is output to the logs(include multiple times for increased debugging information, e.g. -ddd)"`
}

func (opts *Options) SetLogging() {
	userInfoLogger.setVerbosity(opts.Verbosity)
	toolDebugLogger.setVerbosity(opts.Debug)
}
