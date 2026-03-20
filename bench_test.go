package main

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

var names []string

func init() {
	f, err := os.Open("account_names.txt")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			names = append(names, strings.ToLower(normalizeSeparators(line)))
		}
	}
}

func BenchmarkOriginal_SingleWord(b *testing.B) {
	q := tokenSort(normalizeSeparators("blackline"))
	b.ResetTimer()
	for range b.N {
		for _, c := range names {
			jaroWinkler(q, tokenSort(c), defaultPrefixScale)
		}
	}
}

func BenchmarkWordLevel_SingleWord(b *testing.B) {
	q := normalizeSeparators("blackline")
	b.ResetTimer()
	for range b.N {
		for _, c := range names {
			wordLevelScore(q, c, defaultPrefixScale)
		}
	}
}

func BenchmarkOriginal_MultiWord(b *testing.B) {
	q := tokenSort(normalizeSeparators("red cross"))
	b.ResetTimer()
	for range b.N {
		for _, c := range names {
			jaroWinkler(q, tokenSort(c), defaultPrefixScale)
		}
	}
}

func BenchmarkWordLevel_MultiWord(b *testing.B) {
	q := normalizeSeparators("red cross")
	b.ResetTimer()
	for range b.N {
		for _, c := range names {
			wordLevelScore(q, c, defaultPrefixScale)
		}
	}
}
