@echo off
REM Shim so the Go transcriber can invoke whisper_local.py as a single
REM executable (exec.Command picks up the .cmd extension on Windows).
python "%~dp0whisper_local.py" %*
