# active-go

Go binding for Active: pure-Go WASM embedding with zero cgo. Same
evaluator, same guarantees (deterministic, budgeted, secret-redacting).

**Active is the configuration language that refuses to lie.** What you
write is what runs. Units have dimensions; secrets have taint; budgets
always hold.

## Use

```go
import (
    "fmt"
    active "github.com/Active-Official/active-go"
    activewasm "github.com/Active-Official/active-go/wasm"
)

mod, err := active.Load(activewasm.Bytes())
if err != nil { /* fail closed */ }
defer mod.Close()
r, err := mod.Evaluate("port = 8080\n")
// r.OK, r.JSON, r.Diagnostics (codes even when OK carries warnings)
fmt.Println(r.JSON)
```

`Evaluate` mirrors the engine: warnings and strict-collected violations
ride along in `Diagnostics`; a hard failure still returns its codes.
Secrets redact inside the module; never log source without checking.

## Conformance

`go test` runs `conformance/manifest.toml` from an Active checkout (set
`ACTIVE_CONFORMANCE_DIR`; CI checks out both repos). Codes compare
strictly, spans stay advisory, manifest pins record as pins.

## WASM blob

`wasm/active_wasm.wasm` is the pinned release artifact (see `WasmVersion`
in `wasm/embed.go`). It refreshes on every Active release from the
dist bundle after verifying SHA256SUMS.

## License

MIT OR Apache-2.0 (see LICENSE-MIT, LICENSE-APACHE).
