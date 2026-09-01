package canon

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarshalSortsKeysAndOmitsWhitespace(t *testing.T) {
	in := map[string]any{"zeta": 1, "alpha": map[string]any{"y": "a<b>&", "x": []any{1, "s", nil}}}
	got, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"alpha":{"x":[1,"s",null],"y":"a<b>&"},"zeta":1}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestDecodePreservesIntegers(t *testing.T) {
	v, err := Decode([]byte(`{"n":123456789012345678,"p":{"q":-5}}`))
	if err != nil {
		t.Fatal(err)
	}
	n := v.(map[string]any)["n"].(json.Number)
	if n.String() != "123456789012345678" {
		t.Fatalf("integer not preserved: %s", n)
	}
	back, _ := Marshal(v)
	if string(back) != `{"n":123456789012345678,"p":{"q":-5}}` {
		t.Fatalf("round trip changed bytes: %s", back)
	}
}

func TestStripLine(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"abc\n", "abc"}, {"abc\r\n", "abc"}, {"abc", "abc"}, {"abc\r", "abc"},
	} {
		if got := string(StripLine([]byte(c.in))); got != c.want {
			t.Fatalf("StripLine(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestSHA256Hex(t *testing.T) {
	if got := SHA256Hex([]byte("")); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatal(got)
	}
}

func TestMarshalRejectsNonASCIIStringValue(t *testing.T) {
	in := map[string]any{"key": "helloé"} // é is non-ASCII
	_, err := Marshal(in)
	if err == nil {
		t.Fatal("expected error for non-ASCII string value, got nil")
	}
	if !strings.Contains(err.Error(), "non-ASCII rune") {
		t.Fatalf("error message should mention non-ASCII rune: %v", err)
	}
}

func TestMarshalRejectsNonASCIIObjectKey(t *testing.T) {
	in := map[string]any{"é": "value"} // é as key is non-ASCII
	_, err := Marshal(in)
	if err == nil {
		t.Fatal("expected error for non-ASCII object key, got nil")
	}
	if !strings.Contains(err.Error(), "non-ASCII rune") {
		t.Fatalf("error message should mention non-ASCII rune: %v", err)
	}
}

func TestMarshalRejectsNonASCIINestedInSliceAndMap(t *testing.T) {
	// Non-ASCII nested inside a slice inside a map
	in := map[string]any{
		"list": []any{
			"ascii",
			map[string]any{
				"nested": "badévalue",
			},
		},
	}
	_, err := Marshal(in)
	if err == nil {
		t.Fatal("expected error for nested non-ASCII string, got nil")
	}
	if !strings.Contains(err.Error(), "non-ASCII rune") {
		t.Fatalf("error message should mention non-ASCII rune: %v", err)
	}
}

func TestMarshalRejectsControlCharacter(t *testing.T) {
	in := map[string]any{"key": "value\t with\ttab"}
	_, err := Marshal(in)
	if err == nil {
		t.Fatal("expected error for control character, got nil")
	}
	if !strings.Contains(err.Error(), "control character") {
		t.Fatalf("error message should mention control character: %v", err)
	}
}

func TestMarshalAllAcceptedLeafTypesExactBytes(t *testing.T) {
	// Pins the "ASCII output unchanged under the strict contract" claim by
	// exercising every accepted leaf type in one document: string,
	// json.Number, int, int64, bool, nil, plus nested map[string]any and
	// []any containers.
	in := map[string]any{
		"s":   "hello",
		"n":   json.Number("42"),
		"i":   int(7),
		"i64": int64(9007199254740993),
		"b":   true,
		"z":   nil,
		"m":   map[string]any{"k": "v"},
		"a":   []any{1, "x", nil},
	}
	got, err := Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":[1,"x",null],"b":true,"i":7,"i64":9007199254740993,"m":{"k":"v"},"n":42,"s":"hello","z":null}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestMarshalRejectsNonASCIIStringSlice(t *testing.T) {
	in := map[string]any{"tags": []string{"café", "ok"}}
	_, err := Marshal(in)
	if err == nil {
		t.Fatal("expected error for []string value, got nil")
	}
	if !strings.Contains(err.Error(), "[]string") {
		t.Fatalf("error message should name []string: %v", err)
	}
}

func TestMarshalRejectsNonASCIIStringMap(t *testing.T) {
	in := map[string]any{"labels": map[string]string{"k": "café"}}
	_, err := Marshal(in)
	if err == nil {
		t.Fatal("expected error for map[string]string value, got nil")
	}
	if !strings.Contains(err.Error(), "map[string]string") {
		t.Fatalf("error message should name map[string]string: %v", err)
	}
}

func TestMarshalRejectsASCIIOnlyStringSlice(t *testing.T) {
	// []string is refused by type, regardless of whether its content is
	// pure ASCII — the contract is total, not content-dependent.
	in := map[string]any{"tags": []string{"a", "b"}}
	_, err := Marshal(in)
	if err == nil {
		t.Fatal("expected error for []string value even with ASCII-only content, got nil")
	}
	if !strings.Contains(err.Error(), "[]string") {
		t.Fatalf("error message should name []string: %v", err)
	}
}

func TestMarshalRejectsFloat64(t *testing.T) {
	in := map[string]any{"amount": 1.5}
	_, err := Marshal(in)
	if err == nil {
		t.Fatal("expected error for float64 value, got nil")
	}
	if !strings.Contains(err.Error(), "float64") {
		t.Fatalf("error message should name float64: %v", err)
	}
}

func TestMarshalRejectsFloat32(t *testing.T) {
	in := map[string]any{"amount": float32(1.5)}
	_, err := Marshal(in)
	if err == nil {
		t.Fatal("expected error for float32 value, got nil")
	}
	if !strings.Contains(err.Error(), "float32") {
		t.Fatalf("error message should name float32: %v", err)
	}
}

func TestMarshalRejectsStruct(t *testing.T) {
	type point struct{ X, Y int }
	in := map[string]any{"p": point{X: 1, Y: 2}}
	_, err := Marshal(in)
	if err == nil {
		t.Fatal("expected error for struct value, got nil")
	}
	if !strings.Contains(err.Error(), "point") {
		t.Fatalf("error message should name the struct type: %v", err)
	}
}

func TestMarshalIntLeaf(t *testing.T) {
	got, err := Marshal(map[string]any{"n": int(42)})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"n":42}` {
		t.Fatalf("got %s want {\"n\":42}", got)
	}
}

func TestMarshalInt64Leaf(t *testing.T) {
	got, err := Marshal(map[string]any{"n": int64(9007199254740993)})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"n":9007199254740993}` {
		t.Fatalf("got %s want {\"n\":9007199254740993}", got)
	}
}

func TestMarshalJSONNumberBoolNilLeaves(t *testing.T) {
	got, err := Marshal(map[string]any{"n": json.Number("123"), "b": true, "z": nil})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"b":true,"n":123,"z":null}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestMarshalErrorNamesOffendingType(t *testing.T) {
	_, err := Marshal(map[string]any{"bad": []int{1, 2, 3}})
	if err == nil {
		t.Fatal("expected error for []int value, got nil")
	}
	if !strings.Contains(err.Error(), "[]int") {
		t.Fatalf("error message should name []int: %v", err)
	}
}
