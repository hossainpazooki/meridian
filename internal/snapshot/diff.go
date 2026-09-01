package snapshot

import (
	"sort"

	"github.com/hossainpazooki/meridian/internal/canon"
)

// Mismatch is one golden leaf that the doc does not reproduce.
type Mismatch struct{ Path, Want, Got string }

func leafText(v any) string {
	b, err := canon.Marshal(v)
	if err != nil {
		return "<unmarshalable>"
	}
	return string(b)
}

// Diff walks every leaf of golden and compares it to the same path in doc.
// Keys present only in doc are ignored (golden may be a subset). Output is
// sorted by Path.
func Diff(golden, doc Doc) []Mismatch {
	var out []Mismatch
	var walk func(path string, g, d any, present bool)
	walk = func(path string, g, d any, present bool) {
		if gm, ok := g.(map[string]any); ok {
			dm, _ := d.(map[string]any)
			for k, gv := range gm {
				dv, has := dm[k]
				p := k
				if path != "" {
					p = path + "." + k
				}
				walk(p, gv, dv, present && has)
			}
			return
		}
		if !present {
			out = append(out, Mismatch{Path: path, Want: leafText(g), Got: "<absent>"})
			return
		}
		if leafText(g) != leafText(d) {
			out = append(out, Mismatch{Path: path, Want: leafText(g), Got: leafText(d)})
		}
	}
	walk("", golden, doc, true)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Leaves counts non-object leaves in doc (lists count as one leaf).
func Leaves(doc Doc) int {
	c := 0
	var walk func(v any)
	walk = func(v any) {
		if m, ok := v.(map[string]any); ok {
			for _, x := range m {
				walk(x)
			}
			return
		}
		c++
	}
	walk(doc)
	return c
}
