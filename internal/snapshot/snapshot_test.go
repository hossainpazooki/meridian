package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hossainpazooki/meridian/internal/fold"
)

func sample() fold.State {
	return fold.State{Seq: 6, Cash: -76550, RealizedPnL: 17980, DividendIncome: 4500,
		Positions:   map[string]fold.Position{"AAA": {Qty: 180, TotalCost: 99030}, "CCC": {Qty: 3, TotalCost: 1500}},
		Valuations:  map[string]fold.Valuation{"AAA": {Price: 800, PriceSeq: 6, Unrealized: 44970}},
		Unevaluable: []fold.Unevaluable{{Instrument: "CCC", Reason: "no_price_in_prefix"}},
		Absorbed:    []fold.Absorbed{}, Refusals: []fold.Refusal{{Seq: 4, EventID: "ev-4", Key: "k", Kind: "collision", Detail: "payload hash mismatch"}}}
}

func TestBuildIsCanonicalAndStable(t *testing.T) {
	_, b1, h1, err1 := Build(sample(), "sha256:abc")
	if err1 != nil {
		t.Fatal(err1)
	}
	_, b2, h2, err2 := Build(sample(), "sha256:abc")
	if err2 != nil {
		t.Fatal(err2)
	}
	if string(b1) != string(b2) || h1 != h2 {
		t.Fatal("not stable")
	}
	want := `{"absorbed":[],"cash":-76550,"dividend_income":4500,"feed_prefix_hash":"sha256:abc","feed_seq":6,` +
		`"positions":{"AAA":{"qty":180,"total_cost":99030,"valuation":{"price":800,"price_seq":6,"unrealized":44970}},` +
		`"CCC":{"qty":3,"total_cost":1500,"valuation":null}},"realized_pnl":17980,` +
		`"refusals":[{"detail":"payload hash mismatch","event_id":"ev-4","key":"k","kind":"collision","seq":4}],` +
		`"unevaluable":[{"instrument":"CCC","reason":"no_price_in_prefix"}],"unrealized_pnl":44970}` + "\n"
	if string(b1) != want {
		t.Fatalf("bytes:\n%s\nwant:\n%s", b1, want)
	}
	if !strings.HasPrefix(h1, "sha256:") || len(h1) != 71 {
		t.Fatal(h1)
	}
	dir := t.TempDir()
	p, err := Write(dir, b1, h1)
	if err != nil || filepath.Base(p) != strings.TrimPrefix(h1, "sha256:")+".json" {
		t.Fatal(p, err)
	}
	raw, _ := os.ReadFile(p)
	if string(raw) != string(b1) {
		t.Fatal("written bytes differ")
	}
}

func TestDiffReportsLeafMismatchesOnly(t *testing.T) {
	doc, _, _, err := Build(sample(), "sha256:abc")
	if err != nil {
		t.Fatal(err)
	}
	golden, _ := Decode([]byte(`{"cash":-76550,"positions":{"AAA":{"qty":181,"valuation":{"unrealized":44970}},"ZZZ":{"qty":1}},"realized_pnl":17980}`))
	ms := Diff(golden, doc)
	if len(ms) != 2 {
		t.Fatalf("%+v", ms)
	}
	if ms[0].Path != "positions.AAA.qty" || ms[0].Want != "181" || ms[0].Got != "180" {
		t.Fatalf("%+v", ms[0])
	}
	if ms[1].Path != "positions.ZZZ.qty" || ms[1].Got != "<absent>" {
		t.Fatalf("%+v", ms[1])
	}
	if Leaves(golden) != 5 {
		t.Fatal(Leaves(golden))
	}
	if n := Diff(doc, doc); len(n) != 0 {
		t.Fatal(n)
	}
}

// TestBuildErrorsInsteadOfPanickingOnNonASCIIInstrument exercises the
// content-refusal path directly on fold.State: canon.Marshal refuses
// non-ASCII strings, and Build must surface that as an error, not a panic.
// The recover() turns a regression to panicking into a loud test failure
// instead of aborting the whole package's test run.
func TestBuildErrorsInsteadOfPanickingOnNonASCIIInstrument(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Build panicked instead of returning an error: %v", r)
		}
	}()
	s := sample()
	s.Positions = map[string]fold.Position{"AAÉ": {Qty: 1, TotalCost: 1}}
	_, _, _, err := Build(s, "sha256:abc")
	if err == nil {
		t.Fatal("expected an error for a non-ASCII instrument name, got nil")
	}
}

func TestBuildErrorsInsteadOfPanickingOnNonASCIIEventID(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Build panicked instead of returning an error: %v", r)
		}
	}()
	s := sample()
	s.Refusals = []fold.Refusal{{Seq: 4, EventID: "ev-É", Key: "k", Kind: "collision", Detail: "payload hash mismatch"}}
	_, _, _, err := Build(s, "sha256:abc")
	if err == nil {
		t.Fatal("expected an error for a non-ASCII event id, got nil")
	}
}
