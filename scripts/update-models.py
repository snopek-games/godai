#!/usr/bin/env python3
"""Regenerates addons/godai/chat/models.json from models.dev.

Keeps only the providers named by `models_dev` in profiles.gd, only the models
the settings dialog can list (tool-calling and not deprecated), and only the
fields model_info.gd reads.
"""
import json
import re
import sys
import urllib.request
from pathlib import Path

API_URL = "https://models.dev/api.json"
ROOT = Path(__file__).resolve().parent.parent
PROFILES = ROOT / "addons/godai/chat/profiles.gd"
OUTPUT = ROOT / "addons/godai/chat/models.json"

MODEL_FIELDS = ("id", "name", "release_date", "tool_call", "reasoning", "reasoning_options")


def provider_ids():
    return sorted(set(re.findall(r'models_dev = "([^"]*)"', PROFILES.read_text())))


def trim_model(model):
    trimmed = {k: model[k] for k in MODEL_FIELDS if k in model}
    output_limit = model.get("limit", {}).get("output")
    if output_limit is not None:
        trimmed["limit"] = {"output": output_limit}
    return trimmed


def trim_provider(provider):
    models = {
        model_id: trim_model(model)
        for model_id, model in provider.get("models", {}).items()
        if model.get("tool_call") and model.get("status") != "deprecated"
    }
    return {"models": models}


def main():
    ids = provider_ids()
    request = urllib.request.Request(API_URL, headers={"User-Agent": "godai-update-models"})
    with urllib.request.urlopen(request) as response:
        api = json.load(response)
    out = {i: trim_provider(api[i]) for i in ids if i in api}
    with OUTPUT.open("w") as f:
        json.dump(out, f, indent=1, sort_keys=True)
        f.write("\n")
    print(f"Wrote {OUTPUT.relative_to(ROOT)} ({OUTPUT.stat().st_size} bytes) with providers: {' '.join(out)}")
    missing = [i for i in ids if i not in api]
    if missing:
        print(f"Not found on models.dev: {' '.join(missing)}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
