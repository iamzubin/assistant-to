package metrics

import (
	"regexp"
	"strconv"
	"strings"
)

type ParsedTokenMetrics struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	CostUSD          float64
	Model            string
}

func ParseTokenMetrics(transcript string) *ParsedTokenMetrics {
	if transcript == "" {
		return nil
	}

	pm := &ParsedTokenMetrics{}

	lines := strings.Split(transcript, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if pm.Model == "" {
			if model := extractModel(line); model != "" {
				pm.Model = model
			}
		}

		if pm.PromptTokens == 0 {
			if pt := extractPromptTokens(line); pt > 0 {
				pm.PromptTokens = pt
			}
		}

		if pm.CompletionTokens == 0 {
			if ct := extractCompletionTokens(line); ct > 0 {
				pm.CompletionTokens = ct
			}
		}

		if pm.TotalTokens == 0 {
			if tt := extractTotalTokens(line); tt > 0 {
				pm.TotalTokens = tt
			}
		}

		if pm.CostUSD == 0 {
			if cost := extractCost(line); cost > 0 {
				pm.CostUSD = cost
			}
		}
	}

	if pm.TotalTokens == 0 && pm.PromptTokens > 0 && pm.CompletionTokens > 0 {
		pm.TotalTokens = pm.PromptTokens + pm.CompletionTokens
	}

	return pm
}

func extractModel(line string) string {
	modelPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:model|using)[:\s]+(gemini[\w.-]*|GPT[\w.-]*|claude[\w.-]*)`),
		regexp.MustCompile(`(?i)(?:gemini|GPT|claude)[\w.-]+`),
	}

	for _, pattern := range modelPatterns {
		if match := pattern.FindStringSubmatch(line); len(match) > 1 {
			return strings.TrimSpace(match[1])
		}
	}

	for _, model := range []string{"gemini", "gpt", "claude", "anthropic"} {
		if strings.Contains(strings.ToLower(line), model) {
			return model
		}
	}

	return ""
}

func extractPromptTokens(line string) int {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)prompt\s+tokens?[:\s]*(\d+)`),
		regexp.MustCompile(`(?i)input\s+tokens?[:\s]*(\d+)`),
		regexp.MustCompile(`(?i)(\d+)\s*prompt\s+tokens?`),
		regexp.MustCompile(`(?i)(\d+)\s*input\s+tokens?`),
	}

	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(line); len(match) > 1 {
			if val, err := strconv.Atoi(match[1]); err == nil {
				return val
			}
		}
	}

	return 0
}

func extractCompletionTokens(line string) int {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:completion|output)\s+tokens?[:\s]*(\d+)`),
		regexp.MustCompile(`(?i)(\d+)\s*(?:completion|output)\s+tokens?`),
	}

	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(line); len(match) > 1 {
			if val, err := strconv.Atoi(match[1]); err == nil {
				return val
			}
		}
	}

	return 0
}

func extractTotalTokens(line string) int {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)total\s+tokens?[:\s]*(\d+)`),
		regexp.MustCompile(`(?i)total:?\s*(\d+)`),
		regexp.MustCompile(`(?i)(\d+)\s*total\s*tokens?`),
		regexp.MustCompile(`(?i)tokens\s*total[:\s]*(\d+)`),
	}

	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(line); len(match) > 1 {
			if val, err := strconv.Atoi(match[1]); err == nil && val > 100 {
				return val
			}
		}
	}

	return 0
}

func extractCost(line string) float64 {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)cost[:\s]*\$?\s*([\d.]+)`),
		regexp.MustCompile(`(?i)estimated\s+cost[:\s]*\$?\s*([\d.]+)`),
		regexp.MustCompile(`(?i)\$\s*([\d.]+)`),
		regexp.MustCompile(`(?i)USD\s*([\d.]+)`),
	}

	for _, pattern := range patterns {
		if match := pattern.FindStringSubmatch(line); len(match) > 1 {
			if val, err := strconv.ParseFloat(match[1], 64); err == nil && val > 0 {
				return val
			}
		}
	}

	return 0
}

func CalculateCost(promptTokens, completionTokens int, model string) float64 {
	rates := map[string]struct {
		promptPer1M     float64
		completionPer1M float64
	}{
		"gemini-2.0-flash":  {0.10, 0.40},
		"gemini-1.5-flash":  {0.075, 0.30},
		"gemini-1.5-pro":    {1.25, 5.00},
		"gemini-pro":        {1.25, 5.00},
		"gpt-4o":            {2.50, 10.00},
		"gpt-4o-mini":       {0.15, 0.60},
		"gpt-4-turbo":       {10.00, 30.00},
		"gpt-4":             {30.00, 60.00},
		"gpt-3.5-turbo":     {0.50, 1.50},
		"claude-3-5-sonnet": {3.00, 15.00},
		"claude-3-opus":     {15.00, 75.00},
		"claude-3-sonnet":   {3.00, 15.00},
		"claude-3-haiku":    {0.25, 1.25},
	}

	modelLower := strings.ToLower(model)
	for key, rate := range rates {
		if strings.Contains(modelLower, key) {
			promptCost := float64(promptTokens) / 1_000_000 * rate.promptPer1M
			completionCost := float64(completionTokens) / 1_000_000 * rate.completionPer1M
			return promptCost + completionCost
		}
	}

	defaultRate := struct {
		promptPer1M     float64
		completionPer1M float64
	}{0.50, 2.00}

	promptCost := float64(promptTokens) / 1_000_000 * defaultRate.promptPer1M
	completionCost := float64(completionTokens) / 1_000_000 * defaultRate.completionPer1M
	return promptCost + completionCost
}
