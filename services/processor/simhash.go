package main

import (
	"hash/fnv"
	"math/bits"
	"strings"
)

// hashFeature computes a 64-bit FNV-1a hash of a feature string.
func hashFeature(feature string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(feature))
	return h.Sum64()
}

// extractFeatures generates word tokens, word bigrams, and character 3-grams from normalized text.
func extractFeatures(text string) map[string]int {
	features := make(map[string]int)
	words := strings.Fields(text)
	if len(words) == 0 {
		return features
	}

	// 1. Word tokens
	for _, w := range words {
		features["W:"+w] += 2
	}

	// 2. Word bigrams
	for i := 0; i < len(words)-1; i++ {
		features["B:"+words[i]+"_"+words[i+1]] += 3
	}

	// 3. Sliding character 3-grams
	runes := []rune(text)
	if len(runes) >= 3 {
		for i := 0; i <= len(runes)-3; i++ {
			features["C:"+string(runes[i:i+3])] += 1
		}
	}

	return features
}

// ComputeSimhash computes a 64-bit Simhash fingerprint of the given text.
// Returns int64 to match PostgreSQL BIGINT column.
func ComputeSimhash(text string) int64 {
	features := extractFeatures(text)
	if len(features) == 0 {
		return 0
	}

	var v [64]int
	for f, weight := range features {
		h := hashFeature(f)
		for i := 0; i < 64; i++ {
			bit := (h >> i) & 1
			if bit == 1 {
				v[i] += weight
			} else {
				v[i] -= weight
			}
		}
	}

	var fingerprint uint64
	for i := 0; i < 64; i++ {
		if v[i] > 0 {
			fingerprint |= (uint64(1) << i)
		}
	}

	return int64(fingerprint)
}

// HammingDistance computes the number of differing bits between two 64-bit simhashes.
func HammingDistance(a, b int64) int {
	return bits.OnesCount64(uint64(a) ^ uint64(b))
}
