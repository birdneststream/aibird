package request

type (
	Request struct {
		Url      string
		Method   string
		FileName string
		Headers  []Headers
		Fields   []Fields
		Payload  interface{}
	}

	Headers struct {
		Key   string
		Value string
	}

	Fields struct {
		Key   string
		Value string
	}
)
