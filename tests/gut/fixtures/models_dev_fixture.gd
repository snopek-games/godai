extends RefCounted

## A models.dev api.json cut down to the shapes the catalog and dialog care about.
const API := {
	anthropic = {
		id = "anthropic",
		name = "Anthropic",
		models = {
			"claude-new": {id = "claude-new", name = "Claude New", release_date = "2026-06-01", tool_call = true, reasoning = true,
				limit = {context = 1000000, output = 128000},
				reasoning_options = [{type = "effort", values = ["low", "high"]}, {type = "toggle"}]},
			"claude-old": {id = "claude-old", name = "Claude Old", release_date = "2025-01-01", tool_call = true, reasoning = true,
				limit = {output = 8192},
				reasoning_options = [{type = "budget_tokens", min = 1024}]},
			"claude-plain": {id = "claude-plain", name = "Claude Plain", release_date = "2024-01-01", tool_call = true, reasoning = false,
				limit = {output = 4096}},
			"claude-notools": {id = "claude-notools", name = "No Tools", release_date = "2026-07-01", tool_call = false, reasoning = true},
			"claude-dead": {id = "claude-dead", name = "Claude Dead", release_date = "2026-08-01", tool_call = true, status = "deprecated"},
		},
	},
	openai = {
		id = "openai",
		name = "OpenAI",
		models = {
			"gpt-new": {id = "gpt-new", name = "GPT New", release_date = "2026-05-01", tool_call = true, reasoning = true,
				limit = {output = 128000},
				reasoning_options = [{type = "effort", values = ["none", "low", "medium", "high"]}]},
			"gpt-budget": {id = "gpt-budget", name = "GPT Budget", release_date = "2026-04-01", tool_call = true, reasoning = true,
				reasoning_options = [{type = "budget_tokens", min = 128}, {type = "toggle"}]},
		},
	},
	google = {
		id = "google",
		name = "Google",
		models = {
			"gemini-x": {id = "gemini-x", name = "Gemini X", release_date = "2026-03-01", tool_call = true, reasoning = true,
				reasoning_options = [{type = "effort", values = ["low", "medium", "high"]}, {type = "toggle"}]},
		},
	},
	other = {id = "other", name = "Other", models = {}},
}
