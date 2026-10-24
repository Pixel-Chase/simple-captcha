package risk

import "testing"

func TestLeadingZeroBits(t *testing.T) {
	cases := []struct {
		input    []byte
		expected int
	}{
		{[]byte{0x80}, 0},
		{[]byte{0x40}, 1},
		{[]byte{0x01}, 7},
		{[]byte{0x00, 0x80}, 8},
		{[]byte{0x00, 0x00, 0x10}, 19},
		{[]byte{0x00, 0x00}, 16},
	}
	for _, c := range cases {
		if got := LeadingZeroBits(c.input); got != c.expected {
			t.Errorf("LeadingZeroBits(%x) = %d, want %d", c.input, got, c.expected)
		}
	}
}

func TestSolveAndVerifyPow(t *testing.T) {
	nonce, key := "0123456789abcdef0123456789abcdef", "fedcba9876543210fedcba9876543210"

	solution, ok := SolvePow(nonce, key, 12, 0)
	if !ok {
		t.Fatal("expected to find a solution")
	}
	if !VerifyPow(nonce, key, solution, 12) {
		t.Fatal("solution should verify")
	}
	if VerifyPow(nonce, key, solution, 30) {
		t.Error("solution should not satisfy a much higher difficulty")
	}
	if VerifyPow("other-nonce", key, solution, 12) {
		t.Error("solution must be bound to nonce")
	}
	if VerifyPow(nonce, "other-key", solution, 12) {
		t.Error("solution must be bound to key")
	}
}

func TestVerifyPow_EdgeCases(t *testing.T) {
	if !VerifyPow("n", "k", "", 0) {
		t.Error("difficulty 0 means no PoW required")
	}
	if VerifyPow("n", "k", "", 8) {
		t.Error("empty solution must fail")
	}
	long := make([]byte, MaxPowSolutionLength+1)
	for i := range long {
		long[i] = '1'
	}
	if VerifyPow("n", "k", string(long), 1) {
		t.Error("over-long solution must fail")
	}
}
