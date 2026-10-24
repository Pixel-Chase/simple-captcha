package risk

import (
	"crypto/sha256"
	"math/bits"
	"strconv"
)

// MaxPowSolutionLength 解答字符串的最大长度（十进制计数器）
const MaxPowSolutionLength = 32

// PowDigest 计算 PoW 哈希：SHA-256(nonce ":" key ":" solution)。
// key 参与哈希，使得解答只能在拿到本题 key 之后开始计算，无法预先批量生成。
func PowDigest(nonce, key, solution string) [32]byte {
	return sha256.Sum256([]byte(nonce + ":" + key + ":" + solution))
}

// LeadingZeroBits 返回字节序列的前导零比特数
func LeadingZeroBits(b []byte) int {
	count := 0
	for _, v := range b {
		if v == 0 {
			count += 8
			continue
		}
		count += bits.LeadingZeros8(v)
		break
	}
	return count
}

// VerifyPow 校验解答是否满足难度要求。difficulty <= 0 表示本题未要求 PoW。
func VerifyPow(nonce, key, solution string, difficulty int) bool {
	if difficulty <= 0 {
		return true
	}
	if solution == "" || len(solution) > MaxPowSolutionLength {
		return false
	}
	digest := PowDigest(nonce, key, solution)
	return LeadingZeroBits(digest[:]) >= difficulty
}

// SolvePow 以递增计数器暴力搜索满足难度的解答，主要供测试与压测使用。
// maxIterations <= 0 表示不限制次数。
func SolvePow(nonce, key string, difficulty int, maxIterations int) (string, bool) {
	for i := 0; maxIterations <= 0 || i < maxIterations; i++ {
		candidate := strconv.Itoa(i)
		if VerifyPow(nonce, key, candidate, difficulty) {
			return candidate, true
		}
	}
	return "", false
}
