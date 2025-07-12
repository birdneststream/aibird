package servers

type (
	Server struct {
		Host          string
		Port          int
		SSL           bool
		SkipSslVerify bool
		IPv6          bool
	}
)
