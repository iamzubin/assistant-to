package metrics

import (
	"testing"
)

func TestParseTokenMetrics(t *testing.T) {
	tests := []struct {
		name       string
		transcript string
		want       *ParsedTokenMetrics
	}{
		{
			name:       "empty transcript",
			transcript: "",
			want:       nil,
		},
		{
			name:       "Gemini tokens",
			transcript: "Using Gemini 1.5 Pro\nPrompt tokens: 5000\nCompletion tokens: 2000\nTotal tokens: 7000\nCost: $0.05",
			want: &ParsedTokenMetrics{
				PromptTokens:     5000,
				CompletionTokens: 2000,
				TotalTokens:      7000,
				CostUSD:          0.05,
				Model:            "gemini 1.5 pro",
			},
		},
		{
			name:       "GPT tokens with model",
			transcript: "Model: GPT-4\nInput tokens: 3000\nOutput tokens: 1500\nTotal: 4500\nEstimated cost: $0.15",
			want: &ParsedTokenMetrics{
				PromptTokens:     3000,
				CompletionTokens: 1500,
				TotalTokens:      4500,
				CostUSD:          0.15,
				Model:            "gpt-4",
			},
		},
		{
			name:       "Claude tokens",
			transcript: "Claude 3 Sonnet\nPrompt tokens: 2500\nCompletion tokens: 1000\nCost: $0.04",
			want: &ParsedTokenMetrics{
				PromptTokens:     2500,
				CompletionTokens: 1000,
				TotalTokens:      3500,
				CostUSD:          0.04,
				Model:            "claude 3 sonnet",
			},
		},
		{
			name:       "total tokens only",
			transcript: "Total tokens: 10000",
			want: &ParsedTokenMetrics{
				TotalTokens: 10000,
			},
		},
		{
			name:       "cost with dollar sign",
			transcript: "Cost: $2.50",
			want: &ParsedTokenMetrics{
				CostUSD: 2.50,
			},
		},
		{
			name:       "input output tokens",
			transcript: "Input tokens: 4000\nOutput tokens: 800",
			want: &ParsedTokenMetrics{
				PromptTokens:     4000,
				CompletionTokens: 800,
				TotalTokens:      4800,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTokenMetrics(tt.transcript)
			if tt.want == nil {
				if got != nil {
					t.Errorf("ParseTokenMetrics() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Errorf("ParseTokenMetrics() = nil, want %v", tt.want)
				return
			}
			if got.PromptTokens != tt.want.PromptTokens {
				t.Errorf("PromptTokens = %d, want %d", got.PromptTokens, tt.want.PromptTokens)
			}
			if got.CompletionTokens != tt.want.CompletionTokens {
				t.Errorf("CompletionTokens = %d, want %d", got.CompletionTokens, tt.want.CompletionTokens)
			}
			if got.TotalTokens != tt.want.TotalTokens {
				t.Errorf("TotalTokens = %d, want %d", got.TotalTokens, tt.want.TotalTokens)
			}
			if got.CostUSD != tt.want.CostUSD {
				t.Errorf("CostUSD = %v, want %v", got.CostUSD, tt.want.CostUSD)
			}
			if got.Model != tt.want.Model {
				t.Errorf("Model = %q, want %q", got.Model, tt.want.Model)
			}
		})
	}
}

func TestCalculateCost(t *testing.T) {
	tests := []struct {
		name             string
		promptTokens     int
		completionTokens int
		model            string
		wantMin          float64
		wantMax          float64
	}{
		{
			name:             "Gemini 1.5 Flash",
			promptTokens:     1000000,
			completionTokens: 500000,
			model:            "gemini-1.5-flash",
			wantMin:          0.22,
			wantMax:          0.23,
		},
		{
			name:             "GPT-4",
			promptTokens:     1000000,
			completionTokens: 500000,
			model:            "gpt-4",
			wantMin:          55.0,
			wantMax:          65.0,
		},
		{
			name:             "Claude 3.5 Sonnet",
			promptTokens:     1000000,
			completionTokens: 500000,
			model:            "claude-3-5-sonnet",
			wantMin:          10.0,
			wantMax:          11.0,
		},
		{
			name:             "Unknown model uses default rate",
			promptTokens:     1000000,
			completionTokens: 500000,
			model:            "unknown-model",
			wantMin:          1.4,
			wantMax:          1.6,
		},
		{
			name:             "Small tokens",
			promptTokens:     1000,
			completionTokens: 500,
			model:            "gemini-1.5-flash",
			wantMin:          0.0002,
			wantMax:          0.0003,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateCost(tt.promptTokens, tt.completionTokens, tt.model)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("CalculateCost() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}
