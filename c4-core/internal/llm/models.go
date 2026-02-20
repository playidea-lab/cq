package llm

// Catalog is a hardcoded model pricing/spec catalog.
// Used for routing decisions and cost calculations without requiring provider connections.
var Catalog = map[string]ModelInfo{
	"claude-opus-4-6": {
		ID: "claude-opus-4-6", Name: "Claude Opus 4.6",
		ContextWindow: 200000, MaxOutput: 32000,
		InputPer1M: 15.0, OutputPer1M: 75.0,
		SupportsTools: true, SupportsVision: true,
	},
	"claude-sonnet-4-6": {
		ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6",
		ContextWindow: 200000, MaxOutput: 16000,
		InputPer1M: 3.0, OutputPer1M: 15.0,
		SupportsTools: true, SupportsVision: true,
	},
	"claude-sonnet-4-5": {
		ID: "claude-sonnet-4-5", Name: "Claude Sonnet 4.5",
		ContextWindow: 200000, MaxOutput: 16000,
		InputPer1M: 3.0, OutputPer1M: 15.0,
		SupportsTools: true, SupportsVision: true,
	},
	"claude-haiku-4-5-20251001": {
		ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5",
		ContextWindow: 200000, MaxOutput: 8192,
		InputPer1M: 0.80, OutputPer1M: 4.0,
		SupportsTools: true, SupportsVision: true,
	},
	"claude-haiku-3-5": {
		ID: "claude-haiku-3-5", Name: "Claude Haiku 3.5",
		ContextWindow: 200000, MaxOutput: 8192,
		InputPer1M: 0.80, OutputPer1M: 4.0,
		SupportsTools: true, SupportsVision: true,
	},
	"gpt-4-turbo": {
		ID: "gpt-4-turbo", Name: "GPT-4 Turbo",
		ContextWindow: 128000, MaxOutput: 4096,
		InputPer1M: 10.0, OutputPer1M: 30.0,
		SupportsTools: true, SupportsVision: true,
	},
	"gpt-4o": {
		ID: "gpt-4o", Name: "GPT-4o",
		ContextWindow: 128000, MaxOutput: 16384,
		InputPer1M: 2.5, OutputPer1M: 10.0,
		SupportsTools: true, SupportsVision: true,
	},
	"gpt-4o-mini": {
		ID: "gpt-4o-mini", Name: "GPT-4o Mini",
		ContextWindow: 128000, MaxOutput: 16384,
		InputPer1M: 0.15, OutputPer1M: 0.60,
		SupportsTools: true, SupportsVision: true,
	},
	"gemini-2.0-flash": {
		ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash",
		ContextWindow: 1000000, MaxOutput: 8192,
		InputPer1M: 0.10, OutputPer1M: 0.40,
		SupportsTools: true, SupportsVision: true,
	},
	"gemini-2.0-pro": {
		ID: "gemini-2.0-pro", Name: "Gemini 2.0 Pro",
		ContextWindow: 1000000, MaxOutput: 8192,
		InputPer1M: 1.25, OutputPer1M: 10.0,
		SupportsTools: true, SupportsVision: true,
	},
	"llama3.1:70b": {
		ID: "llama3.1:70b", Name: "Llama 3.1 70B (Local)",
		ContextWindow: 128000, MaxOutput: 4096,
		InputPer1M: 0, OutputPer1M: 0,
		SupportsTools: false, SupportsVision: false,
	},
	"text-embedding-3-small": {
		ID: "text-embedding-3-small", Name: "OpenAI Embedding 3 Small",
		ContextWindow: 8191, MaxOutput: 0,
		InputPer1M: 0.02, OutputPer1M: 0,
	},
	"text-embedding-3-large": {
		ID: "text-embedding-3-large", Name: "OpenAI Embedding 3 Large",
		ContextWindow: 8191, MaxOutput: 0,
		InputPer1M: 0.13, OutputPer1M: 0,
	},
}

// Aliases maps short names to full model IDs.
var Aliases = map[string]string{
	"opus":         "claude-opus-4-6",
	"sonnet":       "claude-sonnet-4-6",
	"haiku":        "claude-haiku-4-5-20251001",
	"gpt4":         "gpt-4-turbo",
	"gpt4o":        "gpt-4o",
	"gpt4o-mini":   "gpt-4o-mini",
	"gemini-flash": "gemini-2.0-flash",
	"gemini-pro":   "gemini-2.0-pro",
	"llama70b":     "llama3.1:70b",
}

// ResolveAlias returns the full model ID for an alias, or the input unchanged.
func ResolveAlias(nameOrAlias string) string {
	if full, ok := Aliases[nameOrAlias]; ok {
		return full
	}
	return nameOrAlias
}

// LookupModel returns ModelInfo from the catalog, resolving aliases first.
func LookupModel(nameOrAlias string) (ModelInfo, bool) {
	id := ResolveAlias(nameOrAlias)
	info, ok := Catalog[id]
	return info, ok
}
