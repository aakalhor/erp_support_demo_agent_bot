@echo off
REM Shim so the Go synthesizer can invoke tts_local.py as a single
REM executable (exec.Command honours .cmd extensions on Windows).
python "%~dp0tts_local.py" %*
