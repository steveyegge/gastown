package config

// GroqJSONEnforcement is appended to prompts for groq-compound non-interactive
// invocations to enforce JSON-only output from the compound model.
const GroqJSONEnforcement = "\n\n---\nRESPONSE FORMAT: Respond ONLY with a single " +
	"valid JSON object. No text, markdown, or code fences outside the JSON. " +
	"If unable to comply, return: {\"error\": \"<reason>\"}"
