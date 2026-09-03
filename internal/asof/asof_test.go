package asof

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hossainpazooki/meridian/internal/feed"
)

func TestReadAtViewpointsIsPureRecompute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feed.jsonl")
	f, _ := feed.Open(path)
	f.Append("fill", "e1", "2026-01-05", map[string]any{"instrument": "AAA", "price": json.Number("1000"), "qty": json.Number("100"), "side": "buy", "trade_id": "T-1", "venue": "X"})
	f.Append("action", "e2", "2026-01-10", map[string]any{"action_id": "CA-1", "announced": "2026-01-08", "instrument": "AAA", "kind": "split", "processed": "2026-01-10", "ratio": json.Number("2")})
	f.Append("action_amendment", "e3", "2026-01-10", map[string]any{"action_id": "CA-1", "ratio": json.Number("3")})
	f.Close()

	r1, err := Read(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := Read(path, 2)
	r3, _ := Read(path, -1)
	if r1.State.Positions["AAA"].Qty != 100 || r2.State.Positions["AAA"].Qty != 200 || r3.State.Positions["AAA"].Qty != 300 {
		t.Fatalf("%d %d %d", r1.State.Positions["AAA"].Qty, r2.State.Positions["AAA"].Qty, r3.State.Positions["AAA"].Qty)
	}
	if r3.Seq != 3 || r3.Doc["feed_seq"].(json.Number) != "3" || r3.Doc["feed_prefix_hash"] != r3.PrefixHash {
		t.Fatalf("%+v", r3)
	}
	again, _ := Read(path, -1)
	if again.Hash != r3.Hash || string(again.Bytes) != string(r3.Bytes) {
		t.Fatal("recompute not identical")
	}
	if _, err := Read(path, 4); err == nil {
		t.Fatal("seq beyond feed must error")
	}
}

// TestReadDoesNotPanicOnHostileFeedContent asserts the required end-to-end
// behavior: Read must never panic and must return an error for a feed
// carrying content snapshot.Build cannot render (a non-ASCII instrument
// name). The line below is hand-crafted to bypass feed.Append (which would
// itself refuse this content via the same canon.Marshal check) so it lands
// on disk the way a foreign or tampered producer's file would.
//
// As currently written, feed.Open's own canonicality re-marshal check
// (feed.go's "line failed re-canonicalization" gate) already refuses this
// exact line with a ChainError before fold.Fold or snapshot.Build ever run —
// confirmed by running this test and inspecting the returned error's type.
// So this test's error currently originates at feed.Open, not at
// snapshot.Build. It is kept because the requirement under test is the
// black-box one — Read must not panic and must return an error — and that
// holds regardless of which internal layer catches it; snapshot.Build's own
// fix (returning an error instead of panicking) is what protects any other
// caller that builds a fold.State without going through feed.Open's gate.
func TestReadDoesNotPanicOnHostileFeedContent(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Read panicked on hostile feed content: %v", r)
		}
	}()
	path := filepath.Join(t.TempDir(), "feed.jsonl")
	line := `{"effective":"2026-01-05","id":"e1","payload":{"instrument":"AAÉ","price":1000,"qty":100,"side":"buy","trade_id":"T-1","venue":"X"},"prev":"` +
		feed.Genesis + `","seq":1,"type":"fill"}`
	if err := os.WriteFile(path, []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(path, -1); err == nil {
		t.Fatal("expected Read to return an error for hostile feed content, got nil")
	}
}

func TestReadFromEqualsRead(t *testing.T) {
	path := filepath.Join("..", "..", "fixtures", "base", "feed.jsonl")
	viaPath, err := Read(path, -1)
	if err != nil {
		t.Fatal(err)
	}
	f, err := feed.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	viaFeed, err := ReadFrom(f, -1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(viaPath.Bytes, viaFeed.Bytes) || viaPath.Hash != viaFeed.Hash || viaPath.Seq != viaFeed.Seq {
		t.Fatalf("ReadFrom diverges from Read: %s vs %s", viaPath.Hash, viaFeed.Hash)
	}
	if _, err := ReadFrom(f, f.Len()+1); err == nil {
		t.Fatal("seq past end must error")
	}
}
