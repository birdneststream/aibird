module aibird

go 1.24.4

toolchain go1.24.6

require (
	github.com/BurntSushi/toml v1.6.0
	github.com/dsoprea/go-png-image-structure/v2 v2.0.0-20210512210324-29b889a6093d
	github.com/go-playground/validator/v10 v10.30.1
	github.com/google/uuid v1.6.0
	github.com/hako/durafmt v0.0.0-20210608085754-5c1018a4e16b
	github.com/lrstanley/girc v1.1.1
	github.com/mattn/go-sqlite3 v1.14.33
	github.com/richinsley/comfy2go v0.6.6
	github.com/schollz/progressbar/v3 v3.19.0
	golang.org/x/crypto v0.46.0
)

require (
	github.com/dsoprea/go-exif/v3 v3.0.1 // indirect
	github.com/dsoprea/go-logging v0.0.0-20200710184922-b02d349568dd // indirect
	github.com/dsoprea/go-utility/v2 v2.0.0-20221003172846-a3e1774ef349 // indirect
	github.com/gabriel-vasile/mimetype v1.4.12 // indirect
	github.com/go-errors/errors v1.5.1 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/golang/geo v0.0.0-20251229110840-fd652594c94c // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mitchellh/colorstring v0.0.0-20190213212951-d06e56a500db // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/stretchr/testify v1.11.1 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/term v0.38.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/richinsley/comfy2go => github.com/dreamfast/comfy2go v0.0.0-20260104002633-8fef3ef007cf

replace github.com/lrstanley/girc => github.com/birdneststream/girc v0.0.0-20250828073659-2021c99698d2

replace a2m2a => github.com/birdneststream/a2m2a v0.0.0-20250628223506-d75485e6b2f1
