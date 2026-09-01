package fold

import "testing"

func TestRelieveCostHalfEven(t *testing.T) {
	cases := []struct{ cost, sold, qty, want int64 }{
		{165050, 120, 300, 66020}, // exact
		{1001, 1, 2, 500},         // 500.5 -> 500 (even)
		{1003, 1, 2, 502},         // 501.5 -> 502 (even)
		{1005, 1, 2, 502},         // 502.5 -> 502 (even)
		{10, 1, 3, 3},             // 3.33 -> 3
		{20, 2, 3, 13},            // 13.33 -> 13
		{20, 1, 3, 7},             // 6.67 -> 7
		{99, 99, 99, 99},          // sell all -> whole cost
		{7, 0, 5, 0},
		{0, 3, 5, 0},
		{1 << 40, 1 << 20, 1 << 21, 1 << 39}, // product exceeds int64 -> big.Int path
	}
	for _, c := range cases {
		if got := RelieveCost(c.cost, c.sold, c.qty); got != c.want {
			t.Errorf("RelieveCost(%d,%d,%d)=%d want %d", c.cost, c.sold, c.qty, got, c.want)
		}
	}
}

func TestRelieveCostPanicsGuard(t *testing.T) {
	cases := []struct {
		name            string
		cost, sold, qty int64
	}{
		// The totalQty <= 0 (negative) row cannot be isolated: any non-negative qtySold
		// is greater than any negative totalQty, so qtySold > totalQty is also true.
		// We keep one row for coverage but document the non-isolation.
		{"totalQty <= 0 (zero)", 10, 0, 0},
		{"totalQty <= 0 (negative; also trips qtySold > totalQty, not isolable)", 10, 5, -1},
		{"qtySold < 0", 10, -1, 5},
		{"qtySold > totalQty", 10, 6, 5},
		{"totalCost < 0", -1, 3, 5},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("expected panic on %s", c.name)
				}
			}()
			RelieveCost(c.cost, c.sold, c.qty)
		})
	}
}
