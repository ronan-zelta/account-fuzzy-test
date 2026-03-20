package main

import (
	"bufio"
	"container/heap"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

const defaultPrefixScale = 0.1

var separatorReplacer = strings.NewReplacer("_", " ", "-", " ", ".", " ")

func normalizeSeparators(s string) string {
	if !strings.ContainsAny(s, "_-.'") {
		return s
	}
	s = separatorReplacer.Replace(s)
	return strings.ReplaceAll(s, "'", "")
}

func tokenSort(s string) string {
	words := strings.Fields(s)
	sort.Strings(words)
	return strings.Join(words, " ")
}

func jaroRunes(r1, r2 []rune) float64 {
	len1 := len(r1)
	len2 := len(r2)

	if len1 == 0 || len2 == 0 {
		return 0.0
	}

	matchDist := max(0, max(len1, len2)/2-1)

	matched1 := make([]bool, len1)
	matched2 := make([]bool, len2)

	matches := 0
	transpositions := 0

	for i := range len1 {
		lo := max(0, i-matchDist)
		hi := min(len2, i+matchDist+1)
		for j := lo; j < hi; j++ {
			if matched2[j] || r1[i] != r2[j] {
				continue
			}
			matched1[i] = true
			matched2[j] = true
			matches++
			break
		}
	}

	if matches == 0 {
		return 0.0
	}

	j := 0
	for i := range len1 {
		if !matched1[i] {
			continue
		}
		for !matched2[j] {
			j++
		}
		if r1[i] != r2[j] {
			transpositions++
		}
		j++
	}

	m := float64(matches)
	return (m/float64(len1) + m/float64(len2) + (m-float64(transpositions)/2)/m) / 3.0
}

func jaroWinkler(s1, s2 string, prefixScale float64) float64 {
	if s1 == s2 {
		return 1.0
	}

	r1 := []rune(s1)
	r2 := []rune(s2)

	jaroScore := jaroRunes(r1, r2)

	prefixLen := 0
	maxPrefix := min(4, min(len(r1), len(r2)))
	for i := range maxPrefix {
		if r1[i] != r2[i] {
			break
		}
		prefixLen++
	}

	if prefixScale > 0.25 {
		prefixScale = 0.25
	}

	return jaroScore + float64(prefixLen)*prefixScale*(1.0-jaroScore)
}

// wordLevelScore scores a query against a candidate at the word level.
// Each query word finds its best Jaro-Winkler match among candidate words,
// so prefix boosting happens per-word (e.g. "cross" gets a prefix boost against "crossfit").
// A coverage penalty discounts candidates with many unmatched words.
func wordLevelScore(query, candidate string, prefixScale float64) float64 {
	qWords := strings.Fields(query)
	cWords := strings.Fields(candidate)

	if len(qWords) == 0 || len(cWords) == 0 {
		return 0.0
	}

	// For each query word, find the best matching candidate word
	var totalScore float64
	for _, qw := range qWords {
		bestWord := 0.0
		for _, cw := range cWords {
			s := jaroWinkler(qw, cw, prefixScale)
			if s > bestWord {
				bestWord = s
			}
		}
		totalScore += bestWord
	}

	// Average of best word matches
	avgMatch := totalScore / float64(len(qWords))

	// Mild coverage penalty: if candidate has many extra words, discount slightly.
	// ratio = queryWords / candidateWords, clamped to 1.0
	ratio := float64(len(qWords)) / float64(len(cWords))
	if ratio > 1.0 {
		ratio = 1.0
	}
	// Blend: 80% word match quality + 20% coverage
	return avgMatch*0.8 + avgMatch*ratio*0.2
}

type scored struct {
	name      string
	relevance float64
}

type scoredHeap []scored

func (h scoredHeap) Len() int            { return len(h) }
func (h scoredHeap) Less(i, j int) bool  { return h[i].relevance < h[j].relevance }
func (h scoredHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *scoredHeap) Push(x interface{}) { *h = append(*h, x.(scored)) }
func (h *scoredHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

func main() {
	n := flag.Int("n", 10, "number of results to show")
	file := flag.String("file", "account_names.txt", "path to account names file")
	threshold := flag.Float64("threshold", 0.0, "minimum score threshold")
	mode := flag.String("mode", "hybrid", "scoring mode: 'original', 'word', or 'hybrid' (max of both)")
	flag.Parse()

	query := strings.Join(flag.Args(), " ")
	if query == "" {
		fmt.Fprintf(os.Stderr, "Usage: go run main.go [flags] <search term>\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Load account names
	f, err := os.Open(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening %s: %v\n", *file, err)
		os.Exit(1)
	}
	defer f.Close()

	var names []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			names = append(names, line)
		}
	}

	// Prepare query: lowercase + normalize separators
	queryLower := strings.ToLower(query)
	processed := normalizeSeparators(queryLower)

	var scoreFunc func(processed, candidate string) float64
	switch *mode {
	case "original":
		processed = tokenSort(processed)
		scoreFunc = func(processed, candidate string) float64 {
			return jaroWinkler(processed, tokenSort(candidate), defaultPrefixScale)
		}
	case "word":
		scoreFunc = func(processed, candidate string) float64 {
			return wordLevelScore(processed, candidate, defaultPrefixScale)
		}
	case "hybrid":
		tokenSorted := tokenSort(processed)
		scoreFunc = func(processed, candidate string) float64 {
			w := wordLevelScore(processed, candidate, defaultPrefixScale)
			o := jaroWinkler(tokenSorted, tokenSort(candidate), defaultPrefixScale)
			return max(w, o)
		}
	}

	// Score all names, keep top N via min-heap
	topN := make(scoredHeap, 0, *n+1)
	for _, name := range names {
		candidate := strings.ToLower(name)
		candidate = normalizeSeparators(candidate)
		score := scoreFunc(processed, candidate)
		if score < *threshold {
			continue
		}
		heap.Push(&topN, scored{name: name, relevance: score})
		if topN.Len() > *n {
			heap.Pop(&topN)
		}
	}

	// Sort results descending by relevance
	sort.Slice(topN, func(i, j int) bool {
		return topN[i].relevance > topN[j].relevance
	})

	fmt.Printf("Query: %q  [mode: %s]\n", query, *mode)
	fmt.Printf("Searched %d accounts, showing top %d:\n\n", len(names), len(topN))
	for i, s := range topN {
		fmt.Printf("  %2d. %-40s  (score: %.4f)\n", i+1, s.name, s.relevance)
	}
}
