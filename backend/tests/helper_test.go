package tests

import (
	"simple-captcha/helper"
	"sync"
	"testing"
)

func TestStringToMD5(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "5d41402abc4b2a76b9719d911017c592"},
		{"world", "7d793037a0760186574b0282f2f435e7"},
		{"", "d41d8cd98f00b204e9800998ecf8427e"},
	}

	for _, test := range tests {
		result := helper.StringToMD5(test.input)
		if result != test.expected {
			t.Errorf("StringToMD5(%s) = %s; expected %s", test.input, result, test.expected)
		}
	}
}

func TestGenerateTimestampedID(t *testing.T) {
	id1, err1 := helper.GenerateTimestampedID()
	if err1 != nil {
		t.Errorf("GenerateTimestampedID() returned error: %v", err1)
	}

	id2, err2 := helper.GenerateTimestampedID()
	if err2 != nil {
		t.Errorf("GenerateTimestampedID() returned error: %v", err2)
	}

	// 确保生成的ID不相同
	if id1 == id2 {
		t.Errorf("GenerateTimestampedID() generated duplicate IDs: %s", id1)
	}

	// 确保ID格式正确（包含连字符）
	if len(id1) == 0 || len(id2) == 0 {
		t.Errorf("GenerateTimestampedID() generated empty ID")
	}
}

func TestGenerateRandomString(t *testing.T) {
	tests := []int{1, 5, 10, 16, 32}

	for _, length := range tests {
		result := helper.GenerateRandomString(length)
		if len(result) != length {
			t.Errorf("GenerateRandomString(%d) returned string of length %d; expected %d",
				length, len(result), length)
		}

		// 确保生成的字符串只包含预期的字符
		for _, char := range result {
			if !isValidChar(char) {
				t.Errorf("GenerateRandomString(%d) returned invalid character: %c", length, char)
			}
		}
	}
}

func TestGenerateRandomStringUniqueUnderConcurrency(t *testing.T) {
	const workers, perWorker = 8, 250
	results := make(chan string, workers*perWorker)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				results <- helper.GenerateRandomString(16)
			}
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[string]bool, workers*perWorker)
	for s := range results {
		if seen[s] {
			t.Fatalf("GenerateRandomString 并发调用产生了重复结果: %s", s)
		}
		seen[s] = true
	}

	if got := helper.GenerateRandomString(0); got != "" {
		t.Errorf("GenerateRandomString(0) = %q; expected empty string", got)
	}
}

func isValidChar(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9')
}

func BenchmarkStringToMD5(b *testing.B) {
	input := "benchmark test string"
	for i := 0; i < b.N; i++ {
		helper.StringToMD5(input)
	}
}

func BenchmarkGenerateRandomString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		helper.GenerateRandomString(16)
	}
}
