package domain

import "errors"

// Sentinel errors exposed by the domain so that infrastructure adapters
// can return them and callers can branch on them via errors.Is.
var (
	// ErrIndexNotFound is returned when the persisted search index file
	// does not exist on disk. The API layer translates this into a 503
	// with a hint to run the indexer.
	ErrIndexNotFound = errors.New("search index not found; run the indexer first")

	// ErrSeedNotFound is returned when the seed FAQ file is missing.
	ErrSeedNotFound = errors.New("seed FAQ file not found")

	// ErrInvalidRequest is returned by the use case when the inbound
	// request is structurally invalid (empty question, etc.).
	ErrInvalidRequest = errors.New("invalid request")

	// ErrTranscription is returned by the transcriber when the local
	// whisper command failed. The wrapped error carries the original
	// stderr/exec error for debugging.
	ErrTranscription = errors.New("local transcription failed")
)
