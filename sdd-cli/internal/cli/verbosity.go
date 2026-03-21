package cli

type Verbosity int

const (
	VerbosityQuiet   Verbosity = -1
	VerbosityDefault Verbosity = 0
	VerbosityVerbose Verbosity = 1
	VerbosityDebug   Verbosity = 2
)

func ParseVerbosityFlags(args []string) ([]string, Verbosity) {
	v := VerbosityDefault
	remaining := make([]string, 0, len(args))
	for _, arg := range args {
		switch arg {
		case "-q", "--quiet":
			v = VerbosityQuiet
		case "-v", "--verbose":
			v = VerbosityVerbose
		case "-d", "--debug":
			v = VerbosityDebug
		default:
			remaining = append(remaining, arg)
		}
	}
	return remaining, v
}
