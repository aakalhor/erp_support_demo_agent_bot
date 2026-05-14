#!/usr/bin/env python3
"""
tts_local.py — offline TTS via Meta MMS-TTS (transformers + torch).

Usage:
    tts_local.py --lang en --text "hello world" --out /tmp/out.wav
    tts_local.py --lang pa --text "ਨਮਸਕਾਰ" --out /tmp/out.wav

Models (downloaded on first use, then cached locally):
    en -> facebook/mms-tts-eng
    pa -> facebook/mms-tts-pan

Env overrides:
    TTS_DEVICE       (default: cpu)
    MMS_EN_MODEL_ID  (default: facebook/mms-tts-eng)
    MMS_PA_MODEL_ID  (default: facebook/mms-tts-pan)
"""
import argparse
import os
import sys
from pathlib import Path

LANG_TO_MODEL = {
    "en": os.environ.get("MMS_EN_MODEL_ID", "facebook/mms-tts-eng"),
    "pa": os.environ.get("MMS_PA_MODEL_ID", "facebook/mms-tts-pan"),
}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--lang", required=True, choices=sorted(LANG_TO_MODEL.keys()))
    parser.add_argument("--text", required=True)
    parser.add_argument("--out", required=True)
    args = parser.parse_args()

    text = args.text.strip()
    if not text:
        print("empty text", file=sys.stderr)
        return 2

    model_id = LANG_TO_MODEL[args.lang]
    device = os.environ.get("TTS_DEVICE", "cpu")

    try:
        import torch  # noqa: F401
        from transformers import VitsModel, AutoTokenizer
        import scipy.io.wavfile as wavfile
    except ImportError as e:
        print(
            "MMS-TTS dependencies missing: "
            + str(e)
            + "\nrun: pip install transformers torch scipy",
            file=sys.stderr,
        )
        return 3

    import torch  # imported above; bring into local scope for type checkers

    print(f"[tts_local] lang={args.lang} model={model_id} device={device}", file=sys.stderr)

    tokenizer = AutoTokenizer.from_pretrained(model_id)
    model = VitsModel.from_pretrained(model_id).to(device).eval()

    inputs = tokenizer(text, return_tensors="pt").to(device)
    with torch.no_grad():
        output = model(**inputs).waveform

    audio = output.squeeze().cpu().numpy()
    sample_rate = int(model.config.sampling_rate)

    Path(args.out).parent.mkdir(parents=True, exist_ok=True)
    wavfile.write(args.out, sample_rate, audio)
    print(args.out)
    return 0


if __name__ == "__main__":
    sys.exit(main())
