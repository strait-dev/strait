package schemas

import _ "embed"

// StraitJSON is the authoritative JSON Schema for strait.json configuration files.
// Served at GET /schemas/v1/strait.json.
//
//go:embed strait.json
var StraitJSON []byte
