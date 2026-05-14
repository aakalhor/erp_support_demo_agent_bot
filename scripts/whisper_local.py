#!/usr/bin/env python3
"""
whisper_local.py — CLI wrapper around faster-whisper.

Mimics whisper.cpp's flag shape so the Go transcriber can call it
without code changes:

    whisper_local.py -m <model> -f <wav> -otxt -of <output_stem>

Writes:
    <output_stem>.txt    plain transcript (whisper.cpp compatibility)
    <output_stem>.json   {"transcript": "...", "language": "en", ...}

Also prints the JSON line to stdout so the Go side can parse it.

<model> may be:
  * a faster-whisper model name (e.g. "base.en", "small", "large-v3"),
  * a whisper.cpp ggml filename ("./models/ggml-base.en.bin") — the
    model name is extracted automatically,
  * a CTranslate2 model directory.

Env overrides:
  FASTER_WHISPER_DEVICE         (default: cpu)
  FASTER_WHISPER_COMPUTE_TYPE   (default: int8)
  FASTER_WHISPER_LANGUAGE       (optional explicit language hint)
"""
import argparse
import json
import os
import re
import sys
from pathlib import Path


def resolve_model_id(value: str) -> str:
    p = Path(value)
    if p.is_dir():
        return str(p)
    m = re.match(r"^ggml-(.+)\.bin$", p.name, re.IGNORECASE)
    if m:
        return m.group(1)
    return value


def main() -> int:
    parser = argparse.ArgumentParser(add_help=True)
    parser.add_argument("-m", "--model", required=True)
    parser.add_argument("-f", "--file", required=True)
    parser.add_argument("-otxt", dest="otxt", action="store_true")
    parser.add_argument("-of", "--output-stem", required=True)
    parser.add_argument("--language", default=os.environ.get("FASTER_WHISPER_LANGUAGE") or None)
    args = parser.parse_args()

    if not Path(args.file).exists():
        print(f"audio file not found: {args.file}", file=sys.stderr)
        return 2

    device = os.environ.get("FASTER_WHISPER_DEVICE", "cpu")
    compute_type = os.environ.get("FASTER_WHISPER_COMPUTE_TYPE", "int8")
    model_id = resolve_model_id(args.model)

    try:
        from faster_whisper import WhisperModel
    except ImportError as e:
        print(f"faster-whisper not installed: {e}\nrun: pip install faster-whisper", file=sys.stderr)
        return 3

    print(f"[whisper_local] model={model_id} device={device} compute_type={compute_type} language={args.language or 'auto'}", file=sys.stderr)

    model = WhisperModel(model_id, device=device, compute_type=compute_type)
    segments, info = model.transcribe(
        args.file,
        language=args.language,
        beam_size=1,
        vad_filter=True,
    )
    parts = [seg.text.strip() for seg in segments if seg.text and seg.text.strip()]
    transcript = " ".join(parts).strip()
    detected_lang = (info.language or "").lower() if info is not None else ""
    lang_prob = float(info.language_probability) if info is not None else 0.0

    # whisper.cpp-style .txt for fallback compatibility.
    Path(args.output_stem + ".txt").write_text(transcript, encoding="utf-8")
    payload = {
        "transcript": transcript,
        "language": detected_lang,
        "language_probability": lang_prob,
    }
    Path(args.output_stem + ".json").write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")
    # Print JSON to stdout so the Go transcriber can read it directly.
    print(json.dumps(payload, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
