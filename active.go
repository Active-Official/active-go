// Package active embeds the Active evaluator as WebAssembly and exposes
// an idiomatic Go API with zero cgo. Same evaluator, same guarantees:
// deterministic, budgeted, secret-redacting. See conformance/ in the main
// repo for the shared oracle this binding runs in CI.
package active

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// ABI major this binding was built against. Load refuses any module
// reporting a different major (fail closed, H-002).
const abiMajor = 1

// Result mirrors the WASM JSON envelope.
type Result struct {
	OK          bool
	JSON        string
	Diagnostics string
}

// Module is one instantiated evaluator. Modules are confined to the
// goroutine that uses them; do not share across goroutines without
// external synchronization.
type Module struct {
	rt     wazero.Runtime
	mod    wazero.CompiledModule
	ctx    context.Context
	cancel context.CancelFunc
	api    api.Module
	mem    api.Memory
	alloc  api.Function
	eval   api.Function
	free   api.Function
	abi    api.Function
}

// Load instantiates wasmBytes (an active_wasm.wasm build) and checks the
// ABI major. Pass the embedded build via ActiveWASM from wasm/embed.
func Load(wasmBytes []byte) (*Module, error) {
	ctx, cancel := context.WithCancel(context.Background())
	rt := wazero.NewRuntime(ctx)
	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		cancel()
		rt.Close(ctx)
		return nil, fmt.Errorf("compile: %w", err)
	}
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	if err != nil {
		cancel()
		rt.Close(ctx)
		return nil, fmt.Errorf("instantiate: %w", err)
	}
	m := &Module{rt: rt, mod: compiled, ctx: ctx, cancel: cancel, api: mod, mem: mod.Memory()}
	m.alloc = mod.ExportedFunction("active_alloc")
	m.eval = mod.ExportedFunction("active_eval_json")
	m.free = mod.ExportedFunction("active_free")
	m.abi = mod.ExportedFunction("active_abi_version")
	if m.alloc == nil || m.eval == nil || m.free == nil || m.abi == nil {
		m.Close()
		return nil, errors.New("module missing Active exports")
	}
	out, err := m.abi.Call(ctx)
	if err != nil || len(out) == 0 || out[0] != abiMajor {
		m.Close()
		return nil, fmt.Errorf("ABI major mismatch: want %d", abiMajor)
	}
	return m, nil
}

// Close releases the runtime.
func (m *Module) Close() {
	m.cancel()
	m.api.Close(m.ctx)
	m.rt.Close(m.ctx)
}

func (m *Module) write(s string) (uint32, uint64, error) {
	b := []byte(s)
	out, err := m.alloc.Call(m.ctx, uint64(len(b)))
	if err != nil || len(out) == 0 {
		return 0, 0, fmt.Errorf("alloc: %w", err)
	}
	ptr := uint32(out[0])
	if !m.mem.Write(ptr, b) {
		return 0, 0, errors.New("write out of bounds")
	}
	return ptr, uint64(len(b)), nil
}

func (m *Module) readU32(ptr uint32) (uint64, error) {
	b, ok := m.mem.Read(ptr, 4)
	if !ok {
		return 0, errors.New("read out of bounds")
	}
	return uint64(binary.LittleEndian.Uint32(b)), nil
}

func (m *Module) read(ptr uint32, n uint64) ([]byte, error) {
	b, ok := m.mem.Read(ptr, uint32(n))
	if !ok {
		return nil, errors.New("read out of bounds")
	}
	return b, nil
}

// Evaluate runs source and returns the envelope. Secrets redact inside
// the module; check Diagnostics for codes even when OK is true (warnings
// and strict-collected violations ride along, mirroring the engine).
func (m *Module) Evaluate(source string) (Result, error) {
	var r Result
	sp, sn, err := m.write(source)
	if err != nil {
		return r, err
	}
	defer m.free.Call(m.ctx, uint64(sp), sn)
	slotb, slotn, err := m.write("\x00\x00\x00\x00")
	if err != nil {
		return r, err
	}
	defer m.free.Call(m.ctx, uint64(slotb), slotn)
	out, err := m.eval.Call(m.ctx, uint64(sp), sn, uint64(slotb))
	if err != nil {
		return r, fmt.Errorf("eval trap: %w", err)
	}
	n, err := m.readU32(slotb)
	if err != nil {
		return r, err
	}
	raw, err := m.read(uint32(out[0]), n)
	if err != nil {
		return r, err
	}
	defer m.free.Call(m.ctx, out[0], n)
	var env struct {
		OK   bool   `json:"ok"`
		JSON any    `json:"json"`
		Diag string `json:"diagnostics"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return r, err
	}
	jb, _ := json.Marshal(env.JSON)
	r = Result{OK: env.OK, JSON: string(jb), Diagnostics: env.Diag}
	return r, nil
}
