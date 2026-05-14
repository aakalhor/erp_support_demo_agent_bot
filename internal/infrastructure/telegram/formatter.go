// Package telegram contains the Telegram bot delivery adapter and the
// reply formatter. The bot never embeds business logic; it only stitches
// transport (Telegram), local transcription, and the local /ask API.
package telegram

import (
	"fmt"
	"strings"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
)

// FormatReply renders an AskResponse plus an optional transcript into a
// human-readable Telegram message. The shape matches the spec:
//
//	Transcript:
//	...
//
//	Intent:
//	...
//
//	Answer:
//	...
//
//	Confidence:
//	...
//
//	Escalation:
//	Yes/No
//
//	Sources:
//	- source title, module, score
//
// transcript is optional — pass "" for plain text messages.
func FormatReply(transcript string, resp domain.AskResponse) string {
	var b strings.Builder
	if t := strings.TrimSpace(transcript); t != "" {
		b.WriteString("Transcript:\n")
		b.WriteString(t)
		b.WriteString("\n\n")
	}
	b.WriteString("Intent:\n")
	b.WriteString(string(resp.Intent))
	b.WriteString("\n\nAnswer:\n")
	b.WriteString(resp.Answer)
	b.WriteString("\n\nConfidence:\n")
	b.WriteString(fmt.Sprintf("%.3f", resp.Confidence))
	b.WriteString("\n\nEscalation:\n")
	if resp.EscalationRequired {
		b.WriteString("Yes")
	} else {
		b.WriteString("No")
	}
	b.WriteString("\n\nSources:")
	if len(resp.MatchedSources) == 0 {
		b.WriteString("\n- (no matches)")
	} else {
		for _, s := range resp.MatchedSources {
			b.WriteString(fmt.Sprintf("\n- %s, %s, %.3f", s.SourceTitle, s.Module, s.Score))
		}
	}
	return b.String()
}
