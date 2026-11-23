package ai

import (
	"regexp"
	"strings"

	"github.com/Blinks44/hearth/internal/inventory"
)

// price is a model's API price in US dollars for a million tokens: read
// anew, written out, and read from the prompt cache. Writing to the cache
// costs 1.25 times the input for 5 minutes and twice for an hour.
type price struct{ input, output, cacheRead float64 }

// PricesAsOf is the month the prices are of.
const PricesAsOf = "2026-11"

// prices are the first-party API prices of the models, by normalized ID:
// Anthropic, OpenAI, Google Gemini, xAI Grok, and Moonshot Kimi as of PricesAsOf.
var prices = map[string]price{
	// Anthropic Claude
	"claude-fable-5-1":  {10, 50, 0.25},
	"claude-mythos-5-1": {10, 50, 0.25},
	"claude-fable-5":    {10, 50, 1},
	"claude-mythos-5":   {10, 50, 1},
	"claude-opus-5-5":   {4, 20, 0.2},
	"claude-opus-5":     {5, 25, 0.5},
	"claude-opus-4-8":   {5, 25, 0.5},
	"claude-opus-4-7":   {5, 25, 0.5},
	"claude-opus-4-6":   {5, 25, 0.5},
	"claude-opus-4-5":   {5, 25, 0.5},
	"claude-opus-4-1":   {15, 75, 1.5},
	"claude-opus-4":     {15, 75, 1.5},
	"claude-sonnet-5-5": {2, 10, 0.2},
	"claude-sonnet-5":   {2, 10, 0.2},
	"claude-sonnet-4-6": {3, 15, 0.3},
	"claude-sonnet-4-5": {3, 15, 0.3},
	"claude-sonnet-4":   {3, 15, 0.3},
	"claude-haiku-5-5":  {0.1, 0.5, 0.01},
	"claude-haiku-4-5":  {1, 5, 0.1},
	"claude-3-7-sonnet": {3, 15, 0.3},
	"claude-3-5-sonnet": {3, 15, 0.3},
	"claude-3-5-haiku":  {0.8, 4, 0.08},

	// OpenAI GPT / o-series / Codex
	"gpt-6.2-astra":      {8, 40, 0.4},
	"gpt-6.2-sol":        {2, 10, 0.1},
	"gpt-6-astra":        {10, 50, 1},
	"gpt-6.1-sol":        {2, 10, 0.1},
	"gpt-6.1-terra":      {1, 5, 0.05},
	"gpt-5.4":            {2.5, 15, 0.25},
	"gpt-5.4-mini":       {0.75, 4.5, 0.075},
	"gpt-5.4-nano":       {0.2, 1.25, 0.02},
	"gpt-5.3-codex":      {1.75, 14, 0.175},
	"gpt-5.2-codex":      {1.75, 14, 0.175},
	"gpt-5.2":            {1.75, 14, 0.175},
	"gpt-5.1-codex":      {1.25, 10, 0.125},
	"gpt-5.1-codex-max":  {1.25, 10, 0.125},
	"gpt-5.1-codex-mini": {0.25, 2, 0.025},
	"gpt-5.1":            {1.25, 10, 0.125},
	"gpt-5-codex":        {1.25, 10, 0.125},
	"gpt-5":              {1.25, 10, 0.125},
	"gpt-5-mini":         {0.25, 2, 0.025},
	"gpt-5-nano":         {0.05, 0.4, 0.005},
	"gpt-4.1":            {2, 8, 0.5},
	"gpt-4.1-mini":       {0.4, 1.6, 0.1},
	"gpt-4.1-nano":       {0.1, 0.4, 0.025},
	"gpt-4o":             {2.5, 10, 1.25},
	"gpt-4o-mini":        {0.15, 0.6, 0.075},
	"o3":                 {2, 8, 0.5},
	"o3-mini":            {1.1, 4.4, 0.55},
	"o4-mini":            {1.1, 4.4, 0.275},
	"codex-mini":         {1.5, 6, 0.375},

	// Google Gemini
	"gemini-4-neon":         {3, 15, 0.15},
	"gemini-3.1-pro":        {2, 12, 0.2},
	"gemini-3-pro":          {2, 12, 0.2},
	"gemini-3-flash":        {0.5, 3, 0.05},
	"gemini-2.5-pro":        {1.25, 10, 0.125},
	"gemini-2.5-flash":      {0.3, 2.5, 0.03},
	"gemini-2.5-flash-lite": {0.1, 0.4, 0.01},
	"gemini-2.0-flash":      {0.1, 0.4, 0.025},
	"gemini-2.0-flash-lite": {0.075, 0.3, 0.01875},

	// xAI Grok
	"grok-4":                    {3, 15, 0.75},
	"grok-4-fast":               {0.2, 0.5, 0.05},
	"grok-4-fast-reasoning":     {0.2, 0.5, 0.05},
	"grok-4-fast-non-reasoning": {0.2, 0.5, 0.05},
	"grok-4.1-fast":             {0.2, 0.5, 0.05},
	"grok-code-fast-1":          {0.2, 1.5, 0.02},
	"grok-3":                    {3, 15, 0.75},
	"grok-3-mini":               {0.3, 0.5, 0.075},

	// Moonshot Kimi
	"kimi-k2.5":              {0.6, 3, 0.1},
	"kimi-k2-thinking":       {0.6, 2.5, 0.15},
	"kimi-k2-thinking-turbo": {1.15, 8, 0.15},
	"kimi-k2-turbo":          {1.15, 8, 0.15},
	"kimi-k2":                {0.6, 2.5, 0.15},
	"kimi-for-coding":        {0.6, 3, 0.1},

	// DeepSeek
	"deepseek-chat":     {0.28, 0.42, 0.028},
	"deepseek-reasoner": {0.28, 0.42, 0.028},
	"deepseek-v3.2":     {0.28, 0.42, 0.028},
	"deepseek-v3.1":     {0.28, 0.42, 0.028},
	"deepseek-v3":       {0.27, 1.1, 0.07},
	"deepseek-r1":       {0.55, 2.19, 0.14},

	// Alibaba Qwen
	"qwen3-coder-plus":           {1, 5, 0.1},
	"qwen3-coder":                {1, 5, 0.1},
	"qwen3-coder-flash":          {0.3, 1.5, 0.03},
	"qwen3-max":                  {1.2, 6, 0.24},
	"qwen-max":                   {1.6, 6.4, 0.32},
	"qwen-plus":                  {0.4, 1.2, 0.08},
	"qwen-turbo":                 {0.05, 0.2, 0.01},
	"qwen-flash":                 {0.05, 0.4, 0.01},
	"qwen2.5-coder-32b-instruct": {0.3, 0.9, 0.03},
	"qwen-2.5-coder-32b":         {0.3, 0.9, 0.03},
	"qwq-32b":                    {0.4, 1.2, 0.04},

	// Zhipu / Z.ai GLM
	"glm-5":       {1, 3.2, 0.2},
	"glm-4.7":     {0.6, 2.2, 0.11},
	"glm-4.6":     {0.6, 2.2, 0.11},
	"glm-4.5":     {0.6, 2.2, 0.11},
	"glm-4.5-air": {0.2, 1.1, 0.03},
	"glm-4-plus":  {0.7, 0.7, 0.14},
}

// dated and cloud-provider patterns that model IDs may contain:
//   - "claude-haiku-4-5-20251001"
//   - "gpt-5-2025-08-07"
//   - "gemini-2.5-pro-preview-06-05" / "gemini-3-pro-preview"
//   - "grok-4-0709"
//   - "us.anthropic.claude-sonnet-4-5-20250929-v1:0" (AWS Bedrock)
//   - "publishers/anthropic/models/claude-3-5-sonnet-v2@20241022" (Vertex AI)
var (
	bedrockPrefix  = regexp.MustCompile(`^(?:(?:us|eu|ap|apac|global|au|ca|jp)\.)?(?:anthropic|openai|google|deepseek|qwen|xai|moonshot|zhipu|zai)\.`)
	bedrockVersion = regexp.MustCompile(`-v\d+$`)
	datedCompact   = regexp.MustCompile(`-\d{8}$`)
	datedISO       = regexp.MustCompile(`-\d{4}-\d{2}-\d{2}$`)
	datedShort     = regexp.MustCompile(`-\d{4}$`)
	datedPreview   = regexp.MustCompile(`-preview(?:-\d{2}-\d{2})?$`)
	effortSuffix   = regexp.MustCompile(`-(?:low|medium|high|xhigh)$`)
)

// lookupPrice resolves a raw model ID to its API price, normalizing provider
// prefixes (including AWS Bedrock and Vertex AI), context-window brackets,
// variant suffixes, and release dates.
func lookupPrice(raw string) (price, bool) {
	model := strings.TrimSpace(strings.ToLower(raw))
	model, _, _ = strings.Cut(model, "[") // "claude-opus-5-5[1m]"
	model, _, _ = strings.Cut(model, ":") // "claude-sonnet-4-6:thinking", "...-v1:0"
	if i := strings.LastIndex(model, "/"); i >= 0 {
		model = model[i+1:] // "anthropic/claude-sonnet-4-6", "models/gemini-2.5-pro", "publishers/anthropic/models/..."
	}
	model, _, _ = strings.Cut(model, "@") // Vertex AI: "claude-sonnet-4-5@20250929", "claude-3-5-sonnet-v2@20241022"
	model = bedrockPrefix.ReplaceAllString(model, "")
	if model == "" {
		return price{}, false
	}
	try := func(id string) (price, bool) {
		if p, ok := prices[id]; ok {
			return p, true
		}
		if strings.HasPrefix(id, "claude-") && strings.Contains(id, ".") {
			if p, ok := prices[strings.ReplaceAll(id, ".", "-")]; ok {
				return p, true
			}
		}
		return price{}, false
	}
	stripDates := func(id string) string {
		s := datedCompact.ReplaceAllString(id, "")
		s = datedISO.ReplaceAllString(s, "")
		s = datedPreview.ReplaceAllString(s, "")
		return strings.TrimSuffix(s, "-latest")
	}
	if p, ok := try(model); ok {
		return p, true
	}
	stripped := stripDates(model)
	if p, ok := try(stripped); ok {
		return p, true
	}
	if v := bedrockVersion.ReplaceAllString(stripped, ""); v != stripped {
		v = stripDates(v)
		if p, ok := try(v); ok {
			return p, true
		}
		stripped = v
	}
	if s := datedShort.ReplaceAllString(stripped, ""); s != stripped {
		if p, ok := try(s); ok {
			return p, true
		}
	}
	if s := effortSuffix.ReplaceAllString(stripped, ""); s != stripped {
		if p, ok := try(s); ok {
			return p, true
		}
	}
	return price{}, false
}

// Cost is what u's tokens cost at the API's prices, in US dollars; ok is
// false for a model Hearth has no price of.
func Cost(u inventory.TokenUsage) (usd float64, ok bool) {
	p, ok := lookupPrice(u.Model)
	if !ok {
		return 0, false
	}
	usd = float64(u.Input)*p.input + float64(u.CacheWrite)*p.input*1.25 + float64(u.CacheWrite1h)*p.input*2 +
		float64(u.CacheRead)*p.cacheRead + float64(u.Output)*p.output
	return usd / 1e6, true
}
