package random

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandIntFastStaysInClosedRange(t *testing.T) {
	seen := map[int]bool{}
	for i := 0; i < 2000; i++ {
		v := RandIntFast(-3, 3)
		assert.GreaterOrEqual(t, v, -3)
		assert.LessOrEqual(t, v, 3)
		seen[v] = true
	}
	assert.Len(t, seen, 7, "闭区间两端都应能取到")

	assert.Equal(t, 5, RandIntFast(5, 5))
	assert.GreaterOrEqual(t, RandIntFast(9, 1), 1, "min > max 时应交换")
}

func TestRandNBounds(t *testing.T) {
	for i := 0; i < 1000; i++ {
		assert.Less(t, Rand63n(10), int64(10))
		assert.Less(t, Rand31n(10), int32(10))
	}
	assert.Zero(t, Rand63n(0))
	assert.Zero(t, Rand31n(-1))
}

func TestPermIsPermutation(t *testing.T) {
	p := Perm(50)
	sort.Ints(p)
	for i, v := range p {
		assert.Equal(t, i, v)
	}
	assert.Nil(t, Perm(0))
}
