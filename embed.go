// Package housenka holds files embedded into the binary.
package housenka

import _ "embed"

// ExampleConfig is the commented example configuration written by `housenka config init`.
//
//go:embed config.example.yaml
var ExampleConfig []byte
