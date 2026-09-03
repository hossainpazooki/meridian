// Package asof answers "what did the ledger know at seq V" by replaying the
// prefix [1..V] through the fold on every call. Nothing is cached; nothing
// derived exists outside this call.
package asof

import (
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/fold"
	"github.com/hossainpazooki/meridian/internal/snapshot"
)

type Result struct {
	State            fold.State
	Doc              snapshot.Doc
	Bytes            []byte
	Hash, PrefixHash string
	Seq              int64
}

// Read opens feedPath (verifying its chain), folds [1..seq] and builds the
// snapshot. seq < 0 selects the last record.
func Read(feedPath string, seq int64) (Result, error) {
	f, err := feed.Open(feedPath)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	return ReadFrom(f, seq)
}

// ReadFrom folds [1..seq] of an already-open feed and builds the snapshot.
// seq < 0 selects the last record. It exists so a caller that has already
// opened the feed (to check its length, say) does not pay a second scan.
func ReadFrom(f *feed.Feed, seq int64) (Result, error) {
	if seq < 0 {
		seq = f.Len()
	}
	ph, err := f.PrefixHash(seq)
	if err != nil {
		return Result{}, err
	}
	st, err := fold.Fold(f.Records(), seq)
	if err != nil {
		return Result{}, err
	}
	doc, b, h, err := snapshot.Build(st, ph)
	if err != nil {
		return Result{}, err
	}
	return Result{State: st, Doc: doc, Bytes: b, Hash: h, PrefixHash: ph, Seq: seq}, nil
}
