package token

import "strings"

func EstimateCount(text string) int {
	words := strings.Fields(text)
	total := 0
	for _, w := range words {
		total += len(w)/4 + 1
	}
	return total
}

func EstimateMessages(messages []string) int {
	total := 0
	for _, m := range messages {
		total += EstimateCount(m) + 4
	}
	return total
}

func TruncateToTokens(text string, maxTokens int) string {
	if EstimateCount(text) <= maxTokens {
		return text
	}
	words := strings.Fields(text)
	var result []string
	count := 0
	for _, w := range words {
		wt := len(w)/4 + 1
		if count+wt > maxTokens {
			break
		}
		result = append(result, w)
		count += wt
	}
	return strings.Join(result, " ") + "..."
}
