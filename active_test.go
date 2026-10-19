package active

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	activewasm "github.com/Active-Official/active-go/wasm"
	"github.com/BurntSushi/toml"
)

var codeRe = regexp.MustCompile(`\b([A-Z][0-9]{3})\b`)

func codesOf(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range codeRe.FindAllString(text, -1) {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

type manifestCase struct {
	ID      string `toml:"id"`
	File    string `toml:"file"`
	Kind    string `toml:"kind"`
	Profile string `toml:"profile"`
}

type manifestPin struct {
	ID     string `toml:"id"`
	Reason string `toml:"reason"`
}

type manifest struct {
	Cases []manifestCase `toml:"cases"`
	Pins  []manifestPin  `toml:"pins"`
}

func TestConformance(t *testing.T) {
	conf := os.Getenv("ACTIVE_CONFORMANCE_DIR")
	if conf == "" {
		t.Skip("ACTIVE_CONFORMANCE_DIR unset (CI sets it to the active checkout)")
	}
	var mf manifest
	if _, err := toml.DecodeFile(filepath.Join(conf, "manifest.toml"), &mf); err != nil {
		t.Fatal(err)
	}
	pins := map[string]string{}
	for _, p := range mf.Pins {
		pins[p.ID] = p.Reason
	}
	mod, err := Load(activewasm.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	defer mod.Close()
	readJSON := func(dir, id string) any {
		b, err := os.ReadFile(filepath.Join(conf, dir, id+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, c := range mf.Cases {
		if reason, ok := pins[c.ID]; ok {
			t.Logf("%s PINNED: %s", c.ID, reason)
			continue
		}
		src, err := os.ReadFile(filepath.Join(conf, c.File))
		if err != nil {
			t.Fatal(err)
		}
		r, err := mod.Evaluate(string(src))
		if err != nil {
			t.Fatalf("%s: %v", c.ID, err)
		}
		gotCodes := codesOf(r.Diagnostics)
		if gotCodes == nil {
			gotCodes = []string{}
		}
		switch c.Kind {
		case "clean":
			want := readJSON("expected", c.ID)
			var got any
			if err := json.Unmarshal([]byte(r.JSON), &got); err != nil {
				t.Fatalf("%s: bad json: %v", c.ID, err)
			}
			if !r.OK || len(gotCodes) != 0 || !equalJSON(got, want) {
				t.Errorf("%s: want clean %v, got ok=%v codes=%v json=%s", c.ID, want, r.OK, gotCodes, r.JSON)
			}
		case "diagnosed":
			want := readJSON("expected", c.ID)
			wantCodes := toStrings(readJSON("errors", c.ID).(map[string]any)["codes"])
			var got any
			json.Unmarshal([]byte(r.JSON), &got)
			if !equalJSON(got, want) || !equalStrs(gotCodes, wantCodes) {
				t.Errorf("%s: want %v/%v got %v/%v", c.ID, want, wantCodes, got, gotCodes)
			}
		case "refused":
			wantCodes := toStrings(readJSON("errors", c.ID).(map[string]any)["codes"])
			if !equalStrs(gotCodes, wantCodes) {
				t.Errorf("%s: want codes %v got %v (%s)", c.ID, wantCodes, gotCodes, r.Diagnostics)
			}
		default:
			t.Fatalf("%s: unknown kind %s", c.ID, c.Kind)
		}
	}
}

func equalJSON(a, b any) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func equalStrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
