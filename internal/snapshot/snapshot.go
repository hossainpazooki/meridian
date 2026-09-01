// Package snapshot serializes fold.State as a canonical, content-addressed
// artifact stamped with the feed prefix it derives from. Decode accepts any
// document in the same schema (including Python-emitted twins).
package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hossainpazooki/meridian/internal/canon"
	"github.com/hossainpazooki/meridian/internal/fold"
)

type Doc = map[string]any

func n(i int64) json.Number { return json.Number(strconv.FormatInt(i, 10)) }

// Build renders s. Bytes are canonical JSON plus one '\n'; hash is over those
// bytes. Build returns an error rather than panicking when s carries content
// canon.Marshal refuses (non-ASCII instrument names, event IDs, etc.) —
// malformed input becomes an error here, never a crash, mirroring fold's own
// documented invariant that malformed input becomes a refusal, never a panic.
func Build(s fold.State, prefixHash string) (Doc, []byte, string, error) {
	positions := map[string]any{}
	for inst, p := range s.Positions {
		var val any
		if v, ok := s.Valuations[inst]; ok {
			val = map[string]any{"price": n(v.Price), "price_seq": n(v.PriceSeq), "unrealized": n(v.Unrealized)}
		}
		positions[inst] = map[string]any{"qty": n(p.Qty), "total_cost": n(p.TotalCost), "valuation": val}
	}
	absorbed := make([]any, 0, len(s.Absorbed))
	for _, a := range s.Absorbed {
		absorbed = append(absorbed, map[string]any{"event_id": a.EventID, "key": a.Key, "seq": n(a.Seq)})
	}
	refusals := make([]any, 0, len(s.Refusals))
	for _, r := range s.Refusals {
		refusals = append(refusals, map[string]any{"detail": r.Detail, "event_id": r.EventID, "key": r.Key, "kind": r.Kind, "seq": n(r.Seq)})
	}
	unev := make([]any, 0, len(s.Unevaluable))
	for _, u := range s.Unevaluable {
		unev = append(unev, map[string]any{"instrument": u.Instrument, "reason": u.Reason})
	}
	doc := Doc{
		"absorbed": absorbed, "cash": n(s.Cash), "dividend_income": n(s.DividendIncome),
		"feed_prefix_hash": prefixHash, "feed_seq": n(s.Seq), "positions": positions,
		"realized_pnl": n(s.RealizedPnL), "refusals": refusals, "unevaluable": unev,
		"unrealized_pnl": n(s.UnrealizedPnL()),
	}
	b, err := canon.Marshal(doc)
	if err != nil {
		return nil, nil, "", fmt.Errorf("snapshot: build: %w", err)
	}
	b = append(b, '\n')
	return doc, b, "sha256:" + canon.SHA256Hex(b), nil
}

// Decode parses a snapshot-schema JSON object.
func Decode(b []byte) (Doc, error) {
	v, err := canon.Decode(b)
	if err != nil {
		return nil, err
	}
	d, ok := v.(Doc)
	if !ok {
		return nil, fmt.Errorf("snapshot is not a JSON object")
	}
	return d, nil
}

// Write stores bytes as <dir>/<hex>.json and returns the path.
func Write(dir string, b []byte, hash string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, strings.TrimPrefix(hash, "sha256:")+".json")
	return p, os.WriteFile(p, b, 0o644)
}
