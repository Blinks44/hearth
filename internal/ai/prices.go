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

// prices are the first-party API prices of the models, by ID: Anthropic's
// pricing and OpenAI's as of PricesAsOf.
var prices = map[string]price{
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
	"gpt-6.1-sol":       {2, 10, 0.1},
	"gpt-6.1-terra":     {1, 5, 0.05},
}

// dated is the date some model IDs end in, as "claude-haiku-4-5-20251001".
var dated = regexp.MustCompile(`-\d{8}$`)

// Cost is what u's tokens cost at the API's prices, in US dollars; ok is
// false for a model Hearth has no price of.
func Cost(u inventory.TokenUsage) (usd float64, ok bool) {
	model, _, _ := strings.Cut(strings.ToLower(u.Model), "[") // "claude-opus-5-5[1m]"
	p, ok := prices[dated.ReplaceAllString(model, "")]
	if !ok {
		return 0, false
	}
	usd = float64(u.Input)*p.input + float64(u.CacheWrite)*p.input*1.25 + float64(u.CacheWrite1h)*p.input*2 +
		float64(u.CacheRead)*p.cacheRead + float64(u.Output)*p.output
	return usd / 1e6, true
}
