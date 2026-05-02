package text

import "aibird/birdbase"

// Message is an alias for birdbase.Message to consolidate duplicate structs.
// All code using text.Message transparently uses birdbase.Message.
type Message = birdbase.Message
