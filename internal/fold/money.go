package fold

import "math/big"

// RelieveCost is the ledger's ONLY division: the cost relieved when qtySold of
// totalQty shares are sold from a position carrying totalCost, rounded
// half-to-even. All other money arithmetic is exact integer add/multiply.
// Uses big.Int so the product never overflows; the result always fits int64
// because it is <= totalCost.
func RelieveCost(totalCost, qtySold, totalQty int64) int64 {
	if totalQty <= 0 || qtySold < 0 || qtySold > totalQty || totalCost < 0 {
		panic("fold: RelieveCost invariant violated")
	}
	n := new(big.Int).Mul(big.NewInt(totalCost), big.NewInt(qtySold))
	d := big.NewInt(totalQty)
	quo, rem := new(big.Int).QuoRem(n, d, new(big.Int))
	twice := new(big.Int).Lsh(rem, 1)
	switch twice.Cmp(d) {
	case 1: // remainder > half, round up
		quo.Add(quo, big.NewInt(1))
	case 0: // remainder = half, round to even quotient
		if quo.Bit(0) == 1 {
			quo.Add(quo, big.NewInt(1))
		}
	}
	// case -1 (implicit): remainder < half, rounds down
	return quo.Int64()
}
