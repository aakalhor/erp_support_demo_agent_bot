"""
voice_service.py — persistent local HTTP service hosting:

  * faster-whisper "small" (multilingual) for transcription
  * Meta MMS-TTS English and Punjabi for synthesis

Models are loaded ONCE at process startup, then stay in memory so each
HTTP request only pays for inference.

Run:
    python scripts/voice_service.py

Endpoints (default http://127.0.0.1:7860):
    GET  /health
    POST /transcribe  {audio_path, language?} -> {transcript, language, language_probability}
    POST /speak       {lang, text, out_path}  -> {out_path, sample_rate}

Env overrides:
    VOICE_SERVICE_HOST           (default: 127.0.0.1)
    VOICE_SERVICE_PORT           (default: 7860)
    WHISPER_MODEL_ID             (default: small)
    WHISPER_DEVICE               (default: cpu)
    WHISPER_COMPUTE_TYPE         (default: int8)
    MMS_EN_MODEL_ID              (default: facebook/mms-tts-eng)
    MMS_PA_MODEL_ID              (default: facebook/mms-tts-pan)
    TTS_DEVICE                   (default: cpu)
"""
from __future__ import annotations

import logging
import os
import sys
import time
from pathlib import Path
from typing import Optional

from fastapi import Body, FastAPI, HTTPException
from pydantic import BaseModel

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [voice_service] %(levelname)s %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("voice_service")


# ---------------------------------------------------------------------------
# Request / response models (module scope so FastAPI's type-introspection
# binds them as request bodies, not query parameters).
# ---------------------------------------------------------------------------

class TranscribeReq(BaseModel):
    audio_path: str
    language: Optional[str] = None  # ISO code, or omit for auto-detect


class TranscribeResp(BaseModel):
    transcript: str
    language: str
    language_probability: float


class SpeakReq(BaseModel):
    lang: str
    text: str
    out_path: str


class SpeakResp(BaseModel):
    out_path: str
    sample_rate: int


# ---------------------------------------------------------------------------
# Model loaders
# ---------------------------------------------------------------------------

def _load_whisper():
    from faster_whisper import WhisperModel

    model_id = os.environ.get("WHISPER_MODEL_ID", "small")
    device = os.environ.get("WHISPER_DEVICE", "cpu")
    compute_type = os.environ.get("WHISPER_COMPUTE_TYPE", "int8")
    log.info("loading whisper model=%s device=%s compute_type=%s", model_id, device, compute_type)
    t = time.time()
    m = WhisperModel(model_id, device=device, compute_type=compute_type)
    log.info("whisper loaded in %.1fs", time.time() - t)
    return m


def _load_tts():
    import torch
    from transformers import AutoTokenizer, VitsModel

    device = os.environ.get("TTS_DEVICE", "cpu")
    spec = {
        "en": os.environ.get("MMS_EN_MODEL_ID", "facebook/mms-tts-eng"),
        "pa": os.environ.get("MMS_PA_MODEL_ID", "facebook/mms-tts-pan"),
    }
    bundles = {}
    for lang, model_id in spec.items():
        log.info("loading tts lang=%s model=%s device=%s", lang, model_id, device)
        t = time.time()
        bundles[lang] = {
            "tokenizer": AutoTokenizer.from_pretrained(model_id),
            "model": VitsModel.from_pretrained(model_id).to(device).eval(),
            "device": device,
        }
        log.info("tts lang=%s loaded in %.1fs", lang, time.time() - t)
    return bundles, torch


# ---------------------------------------------------------------------------
# Globals populated at startup
# ---------------------------------------------------------------------------

WHISPER = None
TTS_BUNDLES = None
TORCH = None
WAVFILE = None

app = FastAPI(title="erp-voice-rag voice service", version="1")


@app.get("/health")
def health():
    return {
        "status": "ok",
        "whisper": "ready" if WHISPER is not None else "loading",
        "tts_langs": sorted((TTS_BUNDLES or {}).keys()),
    }


@app.post("/transcribe", response_model=TranscribeResp)
def transcribe(req: TranscribeReq = Body(...)):
    if WHISPER is None:
        raise HTTPException(status_code=503, detail="whisper not ready")
    if not Path(req.audio_path).exists():
        raise HTTPException(status_code=400, detail=f"audio not found: {req.audio_path}")
    t = time.time()
    segments, info = WHISPER.transcribe(
        req.audio_path,
        language=req.language,
        beam_size=1,
        vad_filter=True,
    )
    parts = [s.text.strip() for s in segments if s.text and s.text.strip()]
    transcript = " ".join(parts).strip()
    log.info(
        "transcribe path=%s lang=%s prob=%.2f len=%dch in %.2fs",
        req.audio_path, info.language, info.language_probability,
        len(transcript), time.time() - t,
    )
    return TranscribeResp(
        transcript=transcript,
        language=(info.language or "").lower(),
        language_probability=float(info.language_probability),
    )


@app.post("/speak", response_model=SpeakResp)
def speak(req: SpeakReq = Body(...)):
    if TTS_BUNDLES is None:
        raise HTTPException(status_code=503, detail="tts not ready")
    if req.lang not in TTS_BUNDLES:
        raise HTTPException(status_code=400, detail=f"unsupported lang: {req.lang}")
    text = req.text.strip()
    if not text:
        raise HTTPException(status_code=400, detail="empty text")

    bundle = TTS_BUNDLES[req.lang]
    t = time.time()
    inputs = bundle["tokenizer"](text, return_tensors="pt").to(bundle["device"])
    with TORCH.no_grad():
        output = bundle["model"](**inputs).waveform
    audio = output.squeeze().cpu().numpy()
    sr = int(bundle["model"].config.sampling_rate)

    Path(req.out_path).parent.mkdir(parents=True, exist_ok=True)
    WAVFILE.write(req.out_path, sr, audio)
    log.info(
        "speak lang=%s len=%dch out=%s in %.2fs",
        req.lang, len(text), req.out_path, time.time() - t,
    )
    return SpeakResp(out_path=req.out_path, sample_rate=sr)


def main() -> int:
    try:
        import uvicorn
        import scipy.io.wavfile as wavfile
    except ImportError as e:
        print(f"voice_service deps missing: {e}\nrun: pip install fastapi uvicorn scipy", file=sys.stderr)
        return 3

    global WHISPER, TTS_BUNDLES, TORCH, WAVFILE
    WHISPER = _load_whisper()
    TTS_BUNDLES, TORCH = _load_tts()
    WAVFILE = wavfile

    host = os.environ.get("VOICE_SERVICE_HOST", "127.0.0.1")
    port = int(os.environ.get("VOICE_SERVICE_PORT", "7860"))
    log.info("starting on http://%s:%d", host, port)
    uvicorn.run(app, host=host, port=port, log_level="warning")
    return 0


if __name__ == "__main__":
    sys.exit(main())
