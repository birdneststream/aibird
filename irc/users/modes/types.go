// Package modes defines IRC user mode types.
package modes

type (
	UserModes struct {
		Modes   []string
		Channel string
	}
)
