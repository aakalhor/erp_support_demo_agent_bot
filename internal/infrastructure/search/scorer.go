package search

import "math"

// termCounts converts a token list into term-frequency counts.
func termCounts(tokens []string) map[string]int {
	out := make(map[string]int, len(tokens))
	for _, t := range tokens {
		out[t]++
	}
	return out
}

// idf returns the smoothed inverse-document-frequency for a term given
// the total number of documents N and the document frequency df.
//
//	idf(t) = ln((N + 1) / (df + 1)) + 1
//
// This is the sklearn-style smoothed IDF: it is always strictly positive
// and avoids division by zero when df = 0.
func idf(n, df int) float64 {
	return math.Log(float64(n+1)/float64(df+1)) + 1.0
}

// weightVector turns a token list into a sparse TF-IDF weight vector
// using the provided vocabulary (term -> idf). Terms missing from the
// vocabulary (out-of-vocabulary at query time) are silently dropped —
// they contribute zero to the cosine numerator either way.
func weightVector(tokens []string, vocab map[string]float64) map[string]float64 {
	tf := termCounts(tokens)
	out := make(map[string]float64, len(tf))
	for term, count := range tf {
		idf, ok := vocab[term]
		if !ok {
			continue
		}
		out[term] = float64(count) * idf
	}
	return out
}

// l2Norm returns the Euclidean norm of a sparse weight vector.
func l2Norm(v map[string]float64) float64 {
	var sum float64
	for _, w := range v {
		sum += w * w
	}
	return math.Sqrt(sum)
}

// cosine computes the cosine similarity between two sparse non-negative
// vectors. The returned score lies in [0, 1].
func cosine(a, b map[string]float64, normA, normB float64) float64 {
	if normA == 0 || normB == 0 {
		return 0
	}
	// Iterate the smaller map for speed.
	small, large := a, b
	if len(b) < len(a) {
		small, large = b, a
	}
	var dot float64
	for term, w := range small {
		if other, ok := large[term]; ok {
			dot += w * other
		}
	}
	c := dot / (normA * normB)
	if c < 0 {
		return 0
	}
	if c > 1 {
		return 1
	}
	return c
}
