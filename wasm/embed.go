// Package wasm embeds the released active_wasm.wasm binary.
//
// The blob updates on every Active release: replace active_wasm.wasm with
// the file from the release's dist bundle (checksums in SHA256SUMS) and
// bump WasmVersion below. Never hand-edit the binary.
package wasm

import _ "embed"

//go:embed active_wasm.wasm
var bytes []byte

// WasmVersion is the Active release this blob came from.
const WasmVersion = "1.3.0"

// Bytes returns the embedded module.
func Bytes() []byte { return bytes }
