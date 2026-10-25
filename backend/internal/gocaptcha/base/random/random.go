// 本文件源自 github.com/wenlng/go-captcha v2.0.5（Apache-2.0），已修改，改动见 internal/gocaptcha/NOTICE.md

/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package random

import (
	rand2 "crypto/rand"
	"math"
	"math/big"
	"math/rand/v2"
	"sync"
)

// randPool is a pool of random number generators to avoid lock contention.
//
// 题目答案（拼图位置、点选坐标与校验字、旋转角度）都取自这里，因此每个生成器都是以 crypto/rand
// 为种子的 ChaCha8（密码学安全的伪随机流）。上游以“时间 + math/rand”为种子，而 math/rand v1
// 会把种子对 2^31-1 取模，生成器又在池中复用，攻击者可从已解出的题目反推种子并预测后续答案。
var randPool = sync.Pool{
	New: func() interface{} {
		var seed [32]byte
		if _, err := rand2.Read(seed[:]); err != nil {
			panic("gocaptcha: 读取系统随机源失败: " + err.Error())
		}
		return rand.New(rand.NewChaCha8(seed))
	},
}

// getPooledRnd returns a random number generator from the pool
func getPooledRnd() *rand.Rand {
	return randPool.Get().(*rand.Rand)
}

// putPooledRnd returns a random number generator to the pool
func putPooledRnd(r *rand.Rand) {
	randPool.Put(r)
}

// Rand63n generates a 64-bit random number (thread-safe, high-performance)
func Rand63n(ri int64) int64 {
	if ri <= 0 {
		return 0
	}
	r := getPooledRnd()
	defer putPooledRnd(r)
	return r.Int64N(ri)
}

// Rand31n generates a 32-bit random number (thread-safe, high-performance)
func Rand31n(ri int32) int32 {
	if ri <= 0 {
		return 0
	}
	r := getPooledRnd()
	defer putPooledRnd(r)
	return r.Int32N(ri)
}

// Perm generates a random permutation (thread-safe, high-performance)
func Perm(n int) []int {
	if n <= 0 {
		return nil
	}
	r := getPooledRnd()
	defer putPooledRnd(r)
	return r.Perm(n)
}

// RandInt generates a safe random number in the interval [min, max] (thread-safe)
func RandInt(min, max int) int {
	if min > max {
		min, max = max, min
	}

	if min == max {
		return min
	}

	rangeSize := max - min + 1
	if rangeSize <= 0 {
		return min
	}

	if min < 0 {
		f64Min := math.Abs(float64(min))
		i64Min := int64(f64Min)
		bigRange := big.NewInt(int64(max + 1 + int(i64Min)))
		result, err := rand2.Int(rand2.Reader, bigRange)
		if err != nil {
			// Fallback to math/rand if crypto/rand fails
			return RandIntFast(min, max)
		}
		return int(result.Int64() - i64Min)
	}

	bigRange := big.NewInt(int64(rangeSize))
	result, err := rand2.Int(rand2.Reader, bigRange)
	if err != nil {
		// Fallback to math/rand if crypto/rand fails
		return RandIntFast(min, max)
	}
	return min + int(result.Int64())
}

// RandIntFast generates a random number in the interval [min, max] using the pooled ChaCha8 generators
func RandIntFast(min, max int) int {
	if min > max {
		min, max = max, min
	}

	if min == max {
		return min
	}

	rangeSize := max - min + 1
	if rangeSize <= 0 {
		return min
	}

	r := getPooledRnd()
	defer putPooledRnd(r)
	return min + r.IntN(rangeSize)
}
