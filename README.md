# ERP Voice RAG — Go MVP

A 100% local, bilingual (English + Punjabi), voice-capable ERP support
assistant. Speak or type a question to a Telegram bot; everything from
speech-to-text through retrieval, LLM grounding, safety routing, and
text-to-speech runs on your own machine. No cloud APIs, no hosted
vector DB, no hosted transcription, no external LLM.

The demo is inspired by Acumen Group's public-facing services (Infor
CloudSuite Distribution, Infor Distribution SX.e). It is **not**
affiliated with Acumen Group or Infor and the assistant says so in its
own answers.

---

## Table of contents

- [What it does](#what-it-does)
- [Architecture](#architecture)
- [Clean Architecture and DDD](#clean-architecture-and-ddd)
- [Project layout](#project-layout)
- [Prerequisites](#prerequisites)
- [One-time setup](#one-time-setup)
- [Run](#run)
- [Testing](#testing)
- [Example questions](#example-questions)
- [Safety model](#safety-model)
- [How retrieval scores a match](#how-retrieval-scores-a-match)
- [Where the knowledge comes from](#where-the-knowledge-comes-from)
- [Performance](#performance)
- [Troubleshooting](#troubleshooting)
- [What's intentionally not built](#whats-intentionally-not-built)

---

## What it does

- Telegram bot accepts text and voice messages in English or Punjabi.
- Voice → local ffmpeg → local `faster-whisper small` (multilingual) → transcript + detected language.
- For non-English queries, Qwen translates to English so the retriever can find the right FAQ; the answer is composed back in the user's language.
- Retrieval: TF-IDF cosine over a local JSON index built from a hand-written `seed_faq.jsonl` knowledge base.
- Answer generation: local Ollama (`qwen3:8b` by default). Two prompt modes:
  - **Grounded** when retrieval is confident — the LLM may only use the retrieved FAQs.
  - **Conversational** when retrieval is weak or empty — the LLM handles greetings, off-topic chatter, and "I don't have that answer" gracefully without inventing facts.
- Safety bypass: critical-issue and high-risk topics **never** go to the LLM; they get a curated extractive answer and an escalation flag.
- Text-to-speech: Meta MMS-TTS (`facebook/mms-tts-eng`, `facebook/mms-tts-pan`) → ffmpeg → Telegram voice note. Every reply is sent as both text and voice.

---

## Architecture

```
Telegram (text or voice)
        │
        ▼
┌─────────────────────────┐
│ cmd/bot (Go)            │     supervises ──► python scripts/voice_service.py
│ - downloads voice .ogg  │                    (FastAPI, persistent, 1 process)
│ - ffmpeg ogg→wav        │                    ┌──────────────────────────┐
│ - POST /transcribe ─────┼──────────────────► │ faster-whisper "small"   │
│ - POST /ask             │                    │ + MMS-TTS eng + pan      │
│ - POST /speak  ◄────────┼────────────────────┤ loaded once at startup   │
│ - ffmpeg wav→ogg/opus   │                    └──────────────────────────┘
│ - send text + voice     │
└──────────┬──────────────┘
           │ /ask
           ▼
┌─────────────────────────────────────────────────────────────┐
│ cmd/api (Go)  — POST /ask, GET /health                      │
│                                                             │
│  usecase.AskService                                         │
│   ├── translate question (Qwen via Ollama) ── if non-English│
│   ├── port.SearchIndex (TF-IDF cosine over JSON)            │
│   ├── IntentClassifier (rule cascade)                       │
│   ├── RiskDetector     (rule cascade + corpus risk_level)   │
│   └── route to ONE of:                                      │
│        a) infrastructure/answers/template_generator.go      │
│           (critical / high-risk → curated FAQ, escalate)    │
│        b) infrastructure/answers/ollama_generator.go        │
│           (Qwen3, grounded or conversational by confidence) │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
                   Ollama at 127.0.0.1:11434
                   qwen3:8b (Q4_K_M, ~5 GB)
```

**Everything except Telegram itself runs locally:** `ffmpeg` is a static
binary in `bin/`, the voice service holds Whisper + TTS models in
process memory, the search index is JSON on disk, Qwen runs in Ollama
on `localhost:11434`, and the API is Go `net/http` on `localhost:8080`.

---

## Clean Architecture and DDD

Strict layered dependency direction:

```
domain  →  usecase  ──► port  ◄── infrastructure / delivery
```

- **`internal/domain`** — Pure entities and value objects (`FAQRecord`,
  `Language`, `RiskLevel`, `Intent`, `AskRequest/Response`). No
  third-party imports.
- **`internal/port`** — Interfaces the use case depends on
  (`FAQRepository`, `SearchIndex`, `Transcriber`, `AudioConverter`,
  `AnswerGenerator`, `Translator`, `Synthesizer`). The use case composes
  these; infrastructure provides implementations.
- **`internal/usecase`** — Business logic (`AskService`,
  `IndexService`, `TranscriptionService`, `IntentClassifier`,
  `RiskDetector`). Depends only on `domain` and `port`.
- **`internal/infrastructure`** — Adapters: JSONL repository, TF-IDF
  lexical index, ffmpeg adapter, HTTP transcriber + synthesizer
  (talking to the persistent Python voice service), HTTP delivery
  (Gin-less `net/http` mux), Telegram bot, Ollama client + LLM answer
  generator + LLM translator, supervised Python child process.
- **`cmd/*`** — Tiny entrypoints that wire ports to adapters and start
  the indexer, API, or bot.

DDD touches: `FAQRecord` is an aggregate root with one piece of policy
(`VisibleTo(clientID)`). `RiskLevel`, `Intent`, and `Language` are
closed-enum value objects with normalisation and supported-set checks.
The domain owns its sentinel errors so adapters return them and the
HTTP layer branches on them via `errors.Is`.

---

## Project layout

```
erp-voice-rag-go-mvp/
├── README.md
├── .env.example
├── Makefile
├── go.mod / go.sum
├── data/
│   └── seed_faq.jsonl         # 63 hand-written records
├── storage/                   # index.json (regenerated by cmd/indexer)
├── scripts/
│   ├── voice_service.py       # persistent FastAPI: whisper + 2 MMS-TTS models
│   ├── whisper_local.py       # (legacy CLI wrapper; still works)
│   ├── whisper_local.cmd
│   ├── tts_local.py           # (legacy CLI wrapper; still works)
│   └── tts_local.cmd
├── cmd/
│   ├── api/main.go
│   ├── bot/main.go            # supervises voice_service.py
│   └── indexer/main.go
├── internal/
│   ├── domain/                # entities + value objects + sentinel errors
│   ├── port/                  # interfaces only
│   ├── usecase/
│   ├── infrastructure/
│   │   ├── answers/           # TemplateAnswerGenerator + OllamaAnswerGenerator
│   │   ├── config/
│   │   ├── http/              # /ask + /health handlers
│   │   ├── llm/               # Ollama HTTP client
│   │   ├── repository/        # JSONL FAQ repo
│   │   ├── search/            # tokenizer + TF-IDF + index
│   │   ├── telegram/          # bot + reply formatter
│   │   ├── transcription/     # ffmpeg adapter + HTTP whisper client
│   │   ├── translation/       # Ollama-backed translator
│   │   ├── tts/               # HTTP synthesizer + ffmpeg WAV→OGG
│   │   └── voiceservice/      # supervises the Python voice service
│   └── shared/
│       ├── logger/
│       └── util/
└── tests/
    ├── ask_service_test.go
    ├── lexical_search_test.go
    └── risk_detector_test.go
```

---

## Prerequisites

- **Go 1.22+** (project verified on Go 1.25).
- **Python 3.10+** with `pip`.
- **Ollama** running locally with `qwen3:8b` pulled.
  - Install: <https://ollama.com/download>
  - Pull the model: `ollama pull qwen3:8b`
- **ffmpeg** — a static Windows build is bundled at `bin/ffmpeg.exe` by
  the setup instructions below; on macOS/Linux use `brew install ffmpeg`
  or `apt install ffmpeg`.

You do **not** need to manually install whisper.cpp. The persistent
voice service uses `faster-whisper` (Python). MMS-TTS models are
downloaded automatically on first run.

---

## One-time setup

### 1. Clone and install Go deps

```bash
git clone https://github.com/aakalhor/erp_support_demo_agent_bot.git
cd erp_support_demo_agent_bot
go mod tidy
```

### 2. Install Python deps

```bash
pip install fastapi uvicorn scipy torch transformers faster-whisper
```

If you only have CUDA installed, the default `pip install torch` may
pull a CUDA-enabled wheel. That works too; set `TTS_DEVICE=cuda` or
`WHISPER_DEVICE=cuda` in `.env` to use the GPU.

### 3. Install ffmpeg

- **Windows** (recommended for this MVP): download
  `ffmpeg-release-essentials.zip` from
  <https://www.gyan.dev/ffmpeg/builds/>, extract `ffmpeg.exe`, and place
  it at `bin/ffmpeg.exe`. Set `FFMPEG_PATH` in `.env` to the absolute
  path.
- **macOS**: `brew install ffmpeg` → set `FFMPEG_PATH=ffmpeg`.
- **Linux**: `sudo apt install ffmpeg` → set `FFMPEG_PATH=ffmpeg`.

### 4. Create the Telegram bot

1. Message `@BotFather` on Telegram. Send `/newbot`. Pick a name and a
   unique username ending in `bot`.
2. Copy the HTTP API token.

### 5. Configure `.env`

```bash
cp .env.example .env
```

Open `.env` and set at least:

```
TELEGRAM_BOT_TOKEN=<your token from BotFather>
FFMPEG_PATH=C:\absolute\path\to\bin\ffmpeg.exe   # Windows; or just `ffmpeg` on macOS/Linux
```

The rest (Ollama, voice service paths, etc.) ships with working
defaults. See `.env.example` for the full set.

### 6. Build the search index

```bash
go run ./cmd/indexer
```

You should see:

```
records loaded   : 63
records indexed  : 63
unique client ids: 3
index written to : ./storage/index.json
```

---

## Run

You need **three** local processes: Ollama, the API, and the bot. The
bot will **auto-spawn** the Python voice service on startup (so you do
not need to start it manually).

**Terminal 1** — make sure Ollama is running and has the model:

```bash
ollama serve            # only if not already running as a service
ollama pull qwen3:8b    # one-time
```

**Terminal 2** — start the API:

```bash
# macOS / Linux:
go run ./cmd/api
# Windows PowerShell:
go run .\cmd\api
```

Log lines you should see:

```
ollama OK at http://localhost:11434 (model=qwen3:8b)
listening on :8080 (index: ./storage/index.json)
```

**Terminal 3** — start the bot:

```bash
go run ./cmd/bot
```

The bot spawns `python scripts/voice_service.py` automatically and
waits for it to come up before connecting to Telegram. Expected log:

```
spawning voice service: python ./scripts/voice_service.py
voice service ready at http://127.0.0.1:7860 after ~10s
tts enabled via voice service http://127.0.0.1:7860
telegram bot @yourbot online
```

You can now open your bot in Telegram and start chatting.

To stop everything, press **Ctrl+C** in the bot terminal — the bot
sends `SIGTERM` to the supervised Python service before exiting. If you
ever see an orphan `python.exe` holding port 7860 (e.g. after a hard
kill), kill it manually:

```powershell
# Windows
$p = (Get-NetTCPConnection -LocalPort 7860 -ErrorAction SilentlyContinue).OwningProcess
if ($p) { Stop-Process -Id $p -Force }
```

---

## Testing

### Telegram (full end-to-end)

Send the bot text or voice. Examples that exercise each path:

| What to send                                          | Expected behaviour                                     |
| ----------------------------------------------------- | ------------------------------------------------------ |
| `Hello`                                               | Warm greeting, no escalation, conversational LLM path  |
| `Thanks!`                                             | Polite "you're welcome", no escalation                 |
| `How's the weather?`                                  | Polite deflection back to ERP topics                   |
| `What does your company do?`                          | Grounded LLM answer rephrasing `faq_001`               |
| `Do you support Infor CloudSuite Distribution?`       | Grounded LLM answer rephrasing `faq_006`               |
| `Our ERP system is down`                              | **Critical** — curated template, escalation = yes      |
| `Can I delete a transaction?`                         | **High-risk** — curated template, escalation = yes     |
| `Tell me a poem about the sea`                        | Conversational LLM redirect, no escalation             |
| voice: *"Do you support Infor CloudSuite Distribution?"* | Same as the text version, plus a voice note reply   |
| voice or text in Punjabi: `ਤੁਹਾਡੀ ਕੰਪਨੀ ਕੀ ਕਰਦੀ ਹੈ?`   | Whisper detects `pa`, Qwen answers in Punjabi, MMS-TTS speaks back in Punjabi |
| `ਸਾਡਾ ERP ਸਿਸਟਮ ਡਾਊਨ ਹੈ`                              | Critical bypass, template translated to Punjabi, escalate = yes |

### Direct HTTP (no Telegram / no voice)

```bash
curl -s -X POST http://localhost:8080/ask \
  -H "Content-Type: application/json" \
  -d '{"client_id":"global","question":"Do you support Infor CloudSuite Distribution?"}'
```

A non-English request looks like:

```bash
curl -s -X POST http://localhost:8080/ask \
  -H "Content-Type: application/json; charset=utf-8" \
  --data-binary '{"client_id":"global","question":"ਤੁਹਾਡੀ ਕੰਪਨੀ ਕੀ ਕਰਦੀ ਹੈ?"}'
```

Tip: on Windows shells that mangle UTF-8 in arguments, write the JSON
to a file and use `curl ... -d @body.json` or call from Python with
`urllib.request`.

### Go tests

```bash
go test ./...
```

Covers:

- TF-IDF retrieval returns the right FAQ for the canonical English questions.
- Critical-issue intent detection on phrases like "ERP is down".
- High-risk action detection on phrases like "delete a transaction".
- Client isolation (`client_alpha` never sees `client_beta` records).
- Low-confidence non-critical queries no longer auto-escalate (matches the humanized policy).

---

## Example questions

| Question                                              | Intent                  | Escalates? | Generator path |
| ----------------------------------------------------- | ----------------------- | ---------- | -------------- |
| What does your company do?                            | general_info            | no         | LLM grounded   |
| Do you support Infor CloudSuite Distribution?         | support_request         | no         | LLM grounded   |
| What is Infor Distribution SX.e?                      | general_info            | no         | LLM grounded   |
| What happens during the Plan phase?                   | implementation_question | no         | LLM grounded   |
| What happens during Go-Live?                          | implementation_question | no         | LLM grounded   |
| Can you train our warehouse users?                    | training_question       | no         | LLM grounded   |
| Inventory quantity looks wrong                        | technical_issue         | no         | LLM grounded   |
| Our ERP system is down                                | critical_issue          | **yes**    | Template       |
| Our warehouse users cannot process orders             | critical_issue          | **yes**    | Template       |
| Billing cannot run on tonight's batch                 | critical_issue          | **yes**    | Template       |
| Can I delete a transaction?                           | technical_issue         | **yes**    | Template (high risk) |
| Can I change inventory quantity directly?             | technical_issue         | **yes**    | Template (high risk) |
| Hello / Hi / Thanks / Bye                             | general_info            | no         | LLM conversational |
| Tell me a poem about the sea                          | general_info            | no         | LLM conversational |

---

## Safety model

Layered, in priority order:

1. **Critical phrases** ("system down", "cannot process orders",
   "warehouse blocked", "billing cannot run", "financial posting
   failed", "database error", "all users locked out", "locked out")
   → intent `critical_issue` → always escalates → answer comes from the
   curated template, never the LLM.
2. **High-risk actions** ("delete transaction", "change inventory
   quantity", "modify production configuration", "post financials",
   "overwrite pricing") → risk `high` → always escalates → curated
   template, never LLM.
3. **Medium risk** ("pricing table", "invoice issue", "inventory
   mismatch", "order stuck", "custom report", "user permission",
   "integration issue") → LLM-grounded if a confident FAQ matches, with
   a "review with support if it persists" closing line.
4. **Low confidence / no match** on a safe topic → LLM in
   conversational mode handles small talk, off-topic, or graceful "I
   don't know". Escalation does **not** fire on low confidence alone.
5. **Never invents contact details.** Phrases like "use the critical
   support option on the support line" or "contact your account
   manager" are intentional placeholders. The LLM system prompt forbids
   inventing phone numbers, emails, URLs, names, prices, or policies.

---

## How retrieval scores a match

At index time (`go run ./cmd/indexer`):

1. Each FAQ row is tokenised (lowercase, punctuation stripped,
   stopwords removed). Selected fields are repeated for a static boost:
   `question×3`, `source_title×2`, `product×2`, `module×2`, `tags×2`,
   `answer×1`.
2. Document frequency `df` is counted per term across the corpus.
3. `idf(t) = ln((N+1) / (df+1)) + 1` (sklearn-style smoothed IDF).
4. Each doc's sparse TF-IDF weight vector `w(t,d) = tf(t,d) * idf(t)`
   and its L2 norm are persisted to `storage/index.json`.

At query time (`POST /ask`):

1. The (possibly translated) query is tokenised the same way.
2. Its weight vector is built against the **stored** IDF table.
3. Cosine similarity = `dot(query, doc) / (|query|·|doc|)`, in `[0, 1]`.
4. Visibility filter: `global` queries see only global docs;
   client-specific queries see their own client + global.
5. Top-K = 5 results, sorted descending.

A confidence ≥ 0.55 puts the LLM into grounded mode; below that, into
conversational mode. Critical / high-risk topics always bypass the LLM
entirely.

---

## Where the knowledge comes from

A single hand-written file, **`data/seed_faq.jsonl`** — 63 records:

| Category                                                         | Count |
| ---------------------------------------------------------------- | ----- |
| General services (company, products, hosting, training, etc.)    | 15    |
| Implementation methodology + role-based training                 | 15    |
| ERP support troubleshooting (inventory, billing, orders, …)      | 20    |
| High-risk / critical (ERP down, delete transaction, etc.)        | 10    |
| Client-specific (`client_alpha`, `client_beta`)                  | 3     |

The records are paraphrased demo content I wrote, inspired by Acumen
Group's public-facing services (Infor distribution ERP focus,
implementation phases, role-based training, managed hosting, custom
development). **They are not scraped from any site and the assistant
says so in its own answers.** Every reply the bot produces is either:

- copied verbatim from a matched FAQ (extractive template path), or
- rephrased by Qwen with the matched FAQ as grounding, or
- a conversational reply with explicit instructions not to invent facts.

Each record has these fields:

```json
{
  "id": "faq_001",
  "client_id": "global",
  "company_type": "ERP consulting and support",
  "product": "Infor CloudSuite Distribution",
  "module": "General",
  "question": "What does your company do?",
  "answer": "...",
  "source_title": "Company Overview",
  "source_type": "seed_demo",
  "risk_level": "low",
  "escalation_required": false,
  "tags": ["company", "overview", "services"]
}
```

To customise the knowledge base, edit `data/seed_faq.jsonl` and re-run
the indexer.

---

## Performance

Measured on a Windows 11 box with a recent NVIDIA GPU + CPU int8
quantised Whisper.

| Component                          | Cold-start (one-off) | Warm per call |
| ---------------------------------- | -------------------- | ------------- |
| Voice service load (whisper + 2 TTS) | ~10–15s             | —             |
| Ollama load of `qwen3:8b`          | 30–60s (first call)  | —             |
| `/transcribe` short voice clip     | (covered by cold)    | **~1–2s**     |
| `/speak` English short sentence    | (covered by cold)    | **~1–2s**     |
| `/speak` Punjabi short sentence    | (covered by cold)    | **~0.4–1s**   |
| Qwen3 grounded answer              | —                    | **~0.5–4s**   |
| ffmpeg ogg↔wav                     | —                    | ~0.2s         |

End-to-end Telegram round-trip after warm-up:

- text → text+voice: **1.5–5s**
- voice → text+voice: **3–8s**
- critical bypass (no LLM): **<200ms**

The previous CLI-based design paid ~7–15s **per call** for Python
cold-start. The persistent FastAPI voice service in this version
amortises that cost to a one-time ~10s at startup.

---

## Troubleshooting

| Symptom                                          | Likely cause / fix                                                                                |
| ------------------------------------------------ | ------------------------------------------------------------------------------------------------- |
| `Index not found at ...`                         | Run `go run ./cmd/indexer` first.                                                                 |
| `TELEGRAM_BOT_TOKEN is not set`                  | Fill it in `.env` (see BotFather), restart `cmd/bot`.                                             |
| `ollama health check failed`                     | `ollama serve` is not running, or `qwen3:8b` is not pulled. Run `ollama pull qwen3:8b`.           |
| `voice service did not become ready in ...`      | Check `tmp/voice_service.log`. Common cause: `pip install fastapi uvicorn` not done.              |
| `voice service unreachable at 127.0.0.1:7860`    | Bot crashed leaving an orphan, OR voice service crashed. Kill any stale `python.exe` and restart. |
| Bot reply takes >30s on first message            | First LLM call after Ollama starts loads the 5 GB model. Subsequent calls are fast.               |
| Replies in mixed scripts (Punjabi with stray Latin) | Qwen quirk; safe to ignore for an MVP. Larger Qwen variants reduce this.                        |
| Transcript is empty                              | Speak for 2–3 seconds. Whisper "small" works better on slightly longer clips than `tiny`.         |
| Punjabi voice reply sounds robotic               | MMS-TTS is open-source and CPU-only; quality is what it is. Adequate for the demo.                |

Voice service log lives at `tmp/voice_service.log`; ffmpeg/whisper
errors land there.

---

## What's intentionally not built

- No cloud APIs (OpenAI, AWS, GCP, Azure, hosted vector DB or
  transcription).
- No external database — everything is JSON / JSONL on disk.
- No frontend, no auth, no Docker requirement, no Kubernetes.
- No scraping of Acumen or any other site.
- No phone numbers, emails, or links beyond placeholder text.

---

## License / disclaimer

This is a demo. It is **not** affiliated with Acumen Group or Infor and
the assistant says so in its own answers. ERP terminology used in the
demo (Infor CloudSuite Distribution, Infor Distribution SX.e,
distribution-industry workflows) is generic industry vocabulary.
