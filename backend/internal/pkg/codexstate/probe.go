package codexstate

import (
	"encoding/json"
	"sort"
	"strings"
)

// ProbeBody is the lightweight /responses payload used to harvest Turn-State.
func ProbeBody(model string) []byte {
	if model == "" {
		model = "gpt-5.4"
	}
	body, _ := json.Marshal(map[string]any{
		"model":  model,
		"store":  false,
		"stream": true,
		"input": []map[string]any{{
			"type": "message",
			"role": "user",
			"content": []map[string]any{{
				"type": "input_text",
				"text": ".",
			}},
		}},
	})
	return body
}

// Histogram counts token lengths from a harvest round.
func Histogram(tokens []string) []LenCount {
	counts := map[int]int{}
	for _, tok := range tokens {
		n := len(strings.TrimSpace(tok))
		if n == 0 {
			continue
		}
		counts[n]++
	}
	out := make([]LenCount, 0, len(counts))
	for length, count := range counts {
		out = append(out, LenCount{Len: length, Count: count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Len < out[j].Len })
	return out
}
