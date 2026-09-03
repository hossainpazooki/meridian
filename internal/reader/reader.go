// Package reader is the read seam over Lane 1: the "Reader protocol" the
// design names for Lane 2. A Reader answers three questions -- what is the
// head of the feed, what did the ledger know at seq V, and how does that
// compare to a custodian statement -- and nothing else. The gRPC server
// adapts any Reader; the live one is FeedReader; the P7 twins are Readers
// that lie.
package reader

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/hossainpazooki/meridian/internal/asof"
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/reconcile"
)

var (
	// ErrNotFound: the feed path does not exist. Checked BEFORE feed.Open,
	// which would otherwise create an empty-but-valid feed and answer a
	// clean empty ledger over nothing (the CLI's requireFeedExists rule).
	ErrNotFound = errors.New("feed does not exist")
	// ErrSeqOutOfRange: seq > number of records.
	ErrSeqOutOfRange = errors.New("seq out of range")
	// ErrBadStatement: the statement bytes are not the custodian JSON shape.
	ErrBadStatement = errors.New("bad statement")
)

type Head struct {
	Records    int64
	PrefixHash string
}

type AsOf struct {
	Seq                      int64
	PrefixHash, SnapshotHash string
	Snapshot                 []byte
}

type Recon struct {
	Compared   int64
	Mismatches []reconcile.Mismatch
}

type Reader interface {
	Head(ctx context.Context) (Head, error)
	AsOf(ctx context.Context, seq int64) (AsOf, error)
	Reconcile(ctx context.Context, seq int64, statement []byte) (Recon, error)
}

// FeedReader is the live Reader: every call opens the feed, verifies its
// chain, and recomputes. Nothing is cached, so an append made by another
// process is visible on the next call and a chain break surfaces on read.
type FeedReader struct{ Path string }

func (r FeedReader) exists() error {
	if _, err := os.Stat(r.Path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrNotFound, r.Path)
		}
		return err
	}
	return nil
}

func (r FeedReader) Head(ctx context.Context) (Head, error) {
	if err := r.exists(); err != nil {
		return Head{}, err
	}
	f, err := feed.Open(r.Path)
	if err != nil {
		return Head{}, err
	}
	defer f.Close()
	h, err := f.PrefixHash(f.Len())
	if err != nil {
		return Head{}, err
	}
	return Head{Records: f.Len(), PrefixHash: h}, nil
}

func (r FeedReader) read(seq int64) (asof.Result, error) {
	if err := r.exists(); err != nil {
		return asof.Result{}, err
	}
	f, err := feed.Open(r.Path)
	if err != nil {
		return asof.Result{}, err
	}
	defer f.Close()
	if seq > f.Len() {
		return asof.Result{}, fmt.Errorf("%w: seq %d > records %d", ErrSeqOutOfRange, seq, f.Len())
	}
	return asof.ReadFrom(f, seq)
}

func (r FeedReader) AsOf(ctx context.Context, seq int64) (AsOf, error) {
	res, err := r.read(seq)
	if err != nil {
		return AsOf{}, err
	}
	return AsOf{Seq: res.Seq, PrefixHash: res.PrefixHash, SnapshotHash: res.Hash, Snapshot: res.Bytes}, nil
}

// Reconcile validates the statement first (no I/O) and then reads: a bad
// statement is refused as such even when the feed is also missing.
func (r FeedReader) Reconcile(ctx context.Context, seq int64, statement []byte) (Recon, error) {
	st, err := reconcile.LoadStatementBytes(statement)
	if err != nil {
		return Recon{}, fmt.Errorf("%w: %v", ErrBadStatement, err)
	}
	res, err := r.read(seq)
	if err != nil {
		return Recon{}, err
	}
	ms, compared, err := reconcile.Reconcile(res.Doc, st)
	if err != nil {
		return Recon{}, err
	}
	return Recon{Compared: int64(compared), Mismatches: ms}, nil
}
