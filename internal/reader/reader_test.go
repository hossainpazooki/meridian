package reader

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hossainpazooki/meridian/internal/asof"
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/reconcile"
)

var fixtures = filepath.Join("..", "..", "fixtures")

func base() string { return filepath.Join(fixtures, "base", "feed.jsonl") }

func TestFeedReaderHeadMatchesFeed(t *testing.T) {
	f, err := feed.Open(base())
	if err != nil {
		t.Fatal(err)
	}
	wantHash, _ := f.PrefixHash(f.Len())
	wantLen := f.Len()
	f.Close()
	h, err := FeedReader{Path: base()}.Head(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h.Records != wantLen || h.PrefixHash != wantHash {
		t.Fatalf("Head = %+v, want records=%d prefix=%s", h, wantLen, wantHash)
	}
}

func TestFeedReaderAsOfMatchesAsofRead(t *testing.T) {
	r := FeedReader{Path: base()}
	for _, seq := range []int64{-1, 1} {
		want, err := asof.Read(base(), seq)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.AsOf(context.Background(), seq)
		if err != nil {
			t.Fatalf("seq %d: %v", seq, err)
		}
		if got.Seq != want.Seq || got.PrefixHash != want.PrefixHash || got.SnapshotHash != want.Hash || !bytes.Equal(got.Snapshot, want.Bytes) {
			t.Fatalf("seq %d: AsOf diverges from asof.Read", seq)
		}
	}
}

func TestFeedReaderMissingPathIsNotFoundAndCreatesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.jsonl")
	r := FeedReader{Path: path}
	ctx := context.Background()
	if _, err := r.Head(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Head: got %v, want ErrNotFound", err)
	}
	if _, err := r.AsOf(ctx, -1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AsOf: got %v, want ErrNotFound", err)
	}
	if _, err := r.Reconcile(ctx, -1, []byte(`{"as_of_seq":0,"cash":0,"holdings":[]}`)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Reconcile: got %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a read created %s: feed.Open must never be reached for a missing path", path)
	}
}

func TestFeedReaderSeqPastEndIsOutOfRange(t *testing.T) {
	f, err := feed.Open(base())
	if err != nil {
		t.Fatal(err)
	}
	n := f.Len()
	f.Close()
	r := FeedReader{Path: base()}
	if _, err := r.AsOf(context.Background(), n+1); !errors.Is(err, ErrSeqOutOfRange) {
		t.Fatalf("got %v, want ErrSeqOutOfRange", err)
	}
	if _, err := r.AsOf(context.Background(), n); err != nil {
		t.Fatalf("seq == records must be valid: %v", err)
	}
}

func TestFeedReaderBadStatement(t *testing.T) {
	r := FeedReader{Path: base()}
	for _, raw := range []string{"[]", "{", ""} {
		if _, err := r.Reconcile(context.Background(), -1, []byte(raw)); !errors.Is(err, ErrBadStatement) {
			t.Fatalf("%q: got %v, want ErrBadStatement", raw, err)
		}
	}
}

func TestFeedReaderReconcileMatchesDirect(t *testing.T) {
	stPath := filepath.Join(fixtures, "base", "statement.json")
	raw, err := os.ReadFile(stPath)
	if err != nil {
		t.Fatal(err)
	}
	st, err := reconcile.LoadStatementBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	res, err := asof.Read(base(), -1)
	if err != nil {
		t.Fatal(err)
	}
	wantMs, wantCompared, err := reconcile.Reconcile(res.Doc, st)
	if err != nil {
		t.Fatal(err)
	}
	got, err := FeedReader{Path: base()}.Reconcile(context.Background(), -1, raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Compared != int64(wantCompared) || !reflect.DeepEqual(got.Mismatches, wantMs) {
		t.Fatalf("Reconcile = %+v, want compared=%d ms=%v", got, wantCompared, wantMs)
	}
	if wantCompared == 0 {
		t.Fatal("base statement compared nothing; the fixture is wrong")
	}
}

func TestFeedReaderChainErrorPassesThrough(t *testing.T) {
	tampered := filepath.Join(fixtures, "p2", "tampered", "feed.jsonl")
	_, err := FeedReader{Path: tampered}.Head(context.Background())
	var ce *feed.ChainError
	if !errors.As(err, &ce) {
		t.Fatalf("got %v, want *feed.ChainError", err)
	}
}
