// Package reconcile compares a snapshot to a custodian statement field by
// field. The statement vocabulary (quantity, cost_basis, holdings) is
// deliberately not the snapshot's, so the mapping is explicit and a drift is
// named by instrument, field and signed amount.
package reconcile

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/hossainpazooki/meridian/internal/canon"
	"github.com/hossainpazooki/meridian/internal/snapshot"
)

type Holding struct {
	Instrument          string
	Quantity, CostBasis int64
}
type Statement struct {
	AsOfSeq, Cash int64
	Holdings      []Holding
}
type Mismatch struct {
	Instrument, Field        string
	Ledger, Custodian, Delta int64
}

func asInt(v any) (int64, error) {
	num, ok := v.(json.Number)
	if !ok {
		return 0, fmt.Errorf("not an integer: %v", v)
	}
	return num.Int64()
}

// LoadStatement reads the custodian JSON format from a file.
func LoadStatement(path string) (Statement, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Statement{}, err
	}
	return LoadStatementBytes(raw)
}

// LoadStatementBytes parses the custodian JSON format from bytes. This is
// the one statement schema; the gRPC read API passes statements as bytes
// so it shares this parser rather than carrying a second one.
func LoadStatementBytes(raw []byte) (Statement, error) {
	v, err := canon.Decode(raw)
	if err != nil {
		return Statement{}, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return Statement{}, fmt.Errorf("statement is not an object")
	}
	var st Statement
	if st.AsOfSeq, err = asInt(m["as_of_seq"]); err != nil {
		return Statement{}, fmt.Errorf("as_of_seq: %w", err)
	}
	if st.Cash, err = asInt(m["cash"]); err != nil {
		return Statement{}, fmt.Errorf("cash: %w", err)
	}
	hs, ok := m["holdings"].([]any)
	if !ok {
		return Statement{}, fmt.Errorf("holdings missing")
	}
	for i, h := range hs {
		hm, ok := h.(map[string]any)
		if !ok {
			return Statement{}, fmt.Errorf("holding %d not an object", i)
		}
		inst, ok := hm["instrument"].(string)
		if !ok || inst == "" {
			return Statement{}, fmt.Errorf("holding %d instrument missing", i)
		}
		q, err := asInt(hm["quantity"])
		if err != nil {
			return Statement{}, fmt.Errorf("holding %s quantity: %w", inst, err)
		}
		c, err := asInt(hm["cost_basis"])
		if err != nil {
			return Statement{}, fmt.Errorf("holding %s cost_basis: %w", inst, err)
		}
		st.Holdings = append(st.Holdings, Holding{Instrument: inst, Quantity: q, CostBasis: c})
	}
	return st, nil
}

// Reconcile compares cash and every instrument in the union of both sides.
// It fails closed on a snapshot doc it cannot read — a missing or
// non-integer cash, a missing or non-object positions map, or a position
// that is not an object or is missing/non-integer qty or total_cost — rather
// than defaulting the unreadable value to zero and reporting a fabricated
// mismatch. A document this package cannot read is a document it must
// refuse to judge.
func Reconcile(doc snapshot.Doc, st Statement) ([]Mismatch, int, error) {
	var out []Mismatch
	compared := 0
	cash, err := asInt(doc["cash"])
	if err != nil {
		return nil, 0, fmt.Errorf("doc cash: %w", err)
	}
	compared++
	if cash != st.Cash {
		out = append(out, Mismatch{Field: "cash", Ledger: cash, Custodian: st.Cash, Delta: cash - st.Cash})
	}
	posRaw, ok := doc["positions"].(map[string]any)
	if !ok {
		return nil, 0, fmt.Errorf("doc positions: missing or not an object")
	}
	ledger := map[string][2]int64{}
	for inst, p := range posRaw {
		pm, ok := p.(map[string]any)
		if !ok {
			return nil, 0, fmt.Errorf("doc position %s: not an object", inst)
		}
		q, err := asInt(pm["qty"])
		if err != nil {
			return nil, 0, fmt.Errorf("doc position %s qty: %w", inst, err)
		}
		c, err := asInt(pm["total_cost"])
		if err != nil {
			return nil, 0, fmt.Errorf("doc position %s total_cost: %w", inst, err)
		}
		ledger[inst] = [2]int64{q, c}
	}
	cust := map[string][2]int64{}
	for _, h := range st.Holdings {
		cust[h.Instrument] = [2]int64{h.Quantity, h.CostBasis}
	}
	union := map[string]bool{}
	for k := range ledger {
		union[k] = true
	}
	for k := range cust {
		union[k] = true
	}
	for inst := range union {
		l, c := ledger[inst], cust[inst]
		compared += 2
		if l[0] != c[0] {
			out = append(out, Mismatch{Instrument: inst, Field: "quantity", Ledger: l[0], Custodian: c[0], Delta: l[0] - c[0]})
		}
		if l[1] != c[1] {
			out = append(out, Mismatch{Instrument: inst, Field: "cost_basis", Ledger: l[1], Custodian: c[1], Delta: l[1] - c[1]})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Instrument != out[j].Instrument {
			return out[i].Instrument < out[j].Instrument
		}
		return out[i].Field < out[j].Field
	})
	return out, compared, nil
}
