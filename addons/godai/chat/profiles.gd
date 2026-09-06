extends RefCounted

const AnthropicProvider = preload("res://addons/godai/chat/provider/anthropic.gd")
const OpenAIChatCompletionsProvider = preload("res://addons/godai/chat/provider/openai_chat_completions.gd")

const CUSTOM := "custom"
const DEFAULT := "anthropic"

const PROFILES := {
	anthropic = {
		name = "Anthropic (Claude)",
		models_dev = "anthropic",
		provider = "anthropic",
		url = AnthropicProvider.DEFAULT_URL,
		model = AnthropicProvider.DEFAULT_MODEL,
	},
	openai = {
		name = "OpenAI (ChatGPT)",
		models_dev = "openai",
		provider = "openai_chat_completions",
		url = OpenAIChatCompletionsProvider.DEFAULT_URL,
		model = OpenAIChatCompletionsProvider.DEFAULT_MODEL,
	},
	gemini = {
		name = "Gemini",
		models_dev = "google",
		provider = "openai_chat_completions",
		url = "https://generativelanguage.googleapis.com/v1beta/openai/",
		model = "gemini-2.5-pro",
	},
	ollama = {
		name = "Ollama (Local)",
		provider = "openai_chat_completions",
		url = "http://localhost:11434/v1/",
		model = "",
		use_fake_api_key = true,
	},
}

const FAKE_API_KEY := "fake"


## The models.dev provider id behind a profile, or "" when it has no catalog (or is CUSTOM).
static func models_dev_id(p_profile: String) -> String:
	return str(PROFILES.get(p_profile, {}).get("models_dev", ""))


static func has_catalog(p_profile: String) -> bool:
	return not models_dev_id(p_profile).is_empty()


static func uses_fake_api_key(p_profile: String) -> bool:
	return bool(PROFILES.get(p_profile, {}).get("use_fake_api_key", false))


static func models_dev_ids() -> PackedStringArray:
	var ids := PackedStringArray()
	for id in PROFILES:
		if has_catalog(id):
			ids.push_back(models_dev_id(id))
	return ids


static func matches(p_id: String, p_provider: String, p_url: String) -> bool:
	var profile: Dictionary = PROFILES.get(p_id, {})
	return not profile.is_empty() and profile.provider == p_provider and profile.url.trim_suffix("/") == p_url.trim_suffix("/")


## The profile whose provider and URL are in effect, or CUSTOM.
static func find(p_provider: String, p_url: String) -> String:
	for id in PROFILES:
		if matches(id, p_provider, p_url):
			return String(id)
	return CUSTOM
