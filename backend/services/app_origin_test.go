package services

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeOrigin(t *testing.T) {
	ok := []struct{ in, want string }{
		{"https://www.example.com", "https://www.example.com"},
		{"  HTTPS://WWW.Example.COM  ", "https://www.example.com"},
		{"https://www.example.com/", "https://www.example.com"},
		{"https://www.example.com:443", "https://www.example.com"},
		{"http://www.example.com:80", "http://www.example.com"},
		{"http://localhost:8000", "http://localhost:8000"},
		{"https://example.com:8443", "https://example.com:8443"},
		{"http://127.0.0.1:3321", "http://127.0.0.1:3321"},
		{"http://[::1]:8080", "http://[::1]:8080"},
		{"https://example.com:0443", "https://example.com"},
	}
	for _, tc := range ok {
		got, err := NormalizeOrigin(tc.in)
		require.NoError(t, err, tc.in)
		assert.Equal(t, tc.want, got, tc.in)
	}

	bad := []string{
		"",
		"   ",
		"www.example.com",
		"ftp://example.com",
		"javascript:alert(1)",
		"https://example.com/app",
		"https://example.com/?a=1",
		"https://example.com#x",
		"https://user:pass@example.com",
		"https://*.example.com",
		"https://",
		"https://example.com:",
		"https://example.com:99999",
		"null",
	}
	for _, in := range bad {
		_, err := NormalizeOrigin(in)
		assert.Error(t, err, in)
	}
}

func TestNormalizeOrigins_DedupeAndLimit(t *testing.T) {
	got, cerr := normalizeOrigins([]string{"https://a.com", "HTTPS://A.COM/", "https://a.com:443", "http://a.com"})
	require.Nil(t, cerr)
	assert.Equal(t, []string{"https://a.com", "http://a.com"}, got)

	many := make([]string, 0, 21)
	for i := 0; i < 21; i++ {
		many = append(many, fmt.Sprintf("https://s%d.example.com", i))
	}
	_, cerr = normalizeOrigins(many)
	require.NotNil(t, cerr)
	assert.Contains(t, cerr.Message, "20")

	_, cerr = normalizeOrigins([]string{"https://ok.com", "https://bad.com/path"})
	require.NotNil(t, cerr)
	assert.Contains(t, cerr.Message, "路径")
}
