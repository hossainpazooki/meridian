// Package canon is the single canonical-bytes authority: sorted-key,
// whitespace-free JSON with no HTML escaping, integer-preserving decode,
// sha256 hex, and newline-insensitive line stripping. Both the feed and the
// snapshot derive their hashes from these bytes; nothing else may marshal.
package canon

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Marshal returns canonical JSON: encoding/json sorts map keys bytewise;
// SetEscapeHTML(false) keeps '<', '>', '&' literal. The trailing newline the
// Encoder adds is removed.
//
// Before encoding, Marshal walks the entire value graph and fails closed:
// every node — the root value, every map value, every slice element, and
// every object key — must be exactly one of:
//
//	map[string]any   (keys must be printable ASCII; recurses into values)
//	[]any            (recurses into elements)
//	string           (must be printable ASCII, bytes 0x20..0x7E)
//	json.Number
//	bool
//	nil
//	int, int64
//
// Any other Go type — float32/float64, []string, map[string]string, structs,
// pointers, or any other named or concrete container — is refused with an
// error naming the offending type and its path in the value graph. This is
// a fail-closed contract, not a best-effort one: a type that happens to be
// ASCII-safe and JSON-representable (e.g. []string) is still refused, because
// accepting it by content rather than by type would quietly widen what
// callers can pass. Two concrete reasons this matters:
//
//  1. Non-ASCII strings are refused because Go's encoding/json emits
//     non-ASCII runes as raw UTF-8, while the Python reference implementation
//     escapes them as \uXXXX surrogate pairs — same value, different bytes,
//     different hash.
//  2. floats are refused unconditionally: this project represents all money
//     as integer minor units, and byte-identical replay is load-bearing. A
//     float64 reaching the encoder would serialize in a format the integer
//     contract never intended; refusing it at this boundary enforces the
//     no-floats rule instead of merely documenting it.
func Marshal(v any) ([]byte, error) {
	if err := validate(v, ""); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// validate recursively walks v and returns a non-nil error unless every node
// is one of the types documented on Marshal. path is the location of v in
// the value graph, for error messages (e.g. ".tags" or ".items[2]"); the
// root is passed as "".
func validate(v any, path string) error {
	switch val := v.(type) {
	case nil, bool, json.Number, int, int64:
		return nil
	case string:
		return validateString(val, path)
	case map[string]any:
		for k, mapVal := range val {
			if err := validateString(k, path+"."+k+" (key)"); err != nil {
				return err
			}
			if err := validate(mapVal, path+"."+k); err != nil {
				return err
			}
		}
		return nil
	case []any:
		for i, elem := range val {
			if err := validate(elem, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("canon: unsupported type %T at %s", val, displayPath(path))
	}
}

// displayPath renders path for an error message, substituting "$" for the
// root (path == "").
func displayPath(path string) string {
	if path == "" {
		return "$"
	}
	return path
}

// validateString checks that s contains only printable ASCII (0x20..0x7E).
// Control characters (0x00-0x1F, 0x7F) and non-ASCII runes (0x80+) are
// distinct violations and get distinct error text — a stray tab is not the
// same bug as a stray é, and conflating them under one message misleads
// whoever is debugging the report.
func validateString(s string, path string) error {
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7F:
			return fmt.Errorf("canon: control character %q in string %q at %s", r, s, displayPath(path))
		case r > 0x7E:
			return fmt.Errorf("canon: non-ASCII rune %q in string %q at %s", r, s, displayPath(path))
		}
	}
	return nil
}

// Decode parses JSON into map[string]any / []any / json.Number / string /
// bool / nil. Numbers stay json.Number so integers never pass through float64.
func Decode(b []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// SHA256Hex returns the lowercase hex sha256 of b.
func SHA256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// StripLine removes one trailing '\n' and then one trailing '\r'. Every
// hash over a feed line goes through this so CRLF checkouts hash identically.
func StripLine(b []byte) []byte {
	b = bytes.TrimSuffix(b, []byte("\n"))
	return bytes.TrimSuffix(b, []byte("\r"))
}
