// Package tests holds the cross-package integration tests. Lower-level
// pure-unit tests could also live next to their package; everything is
// in /tests for the MVP so newcomers find them in one place.
package tests

import (
	"testing"

	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/domain"
	"github.com/acuman-demo/erp-voice-rag-go-mvp/internal/infrastructure/search"
)

// fixtureCorpus is a small hand-written corpus that mirrors the shape
// of the real seed file. Keeping the tests self-contained makes them
// fast and immune to seed edits.
func fixtureCorpus() []domain.FAQRecord {
	return []domain.FAQRecord{
		{
			ID: "g_csd", ClientID: domain.ClientGlobal,
			Product: "Infor CloudSuite Distribution", Module: "General",
			Question: "Do you support Infor CloudSuite Distribution?",
			Answer:   "Yes. Infor CloudSuite Distribution (CSD) is a core supported product covering implementation, configuration, custom development, and support.",
			Tags:     []string{"csd", "cloudsuite", "infor"},
			RiskLevel: domain.RiskLow,
		},
		{
			ID: "g_sxe", ClientID: domain.ClientGlobal,
			Product: "Infor Distribution SX.e", Module: "General",
			Question: "What is Infor Distribution SX.e?",
			Answer:   "SX.e is an enterprise distribution ERP covering order entry, inventory, purchasing, warehouse operations, pricing, and financials.",
			Tags:     []string{"sxe", "infor", "distribution"},
			RiskLevel: domain.RiskLow,
		},
		{
			ID: "g_down", ClientID: domain.ClientGlobal,
			Product: "General", Module: "Support",
			Question: "Our ERP system is down",
			Answer:   "An ERP-down event is treated as a critical issue. Capture timing and affected users, then use the critical support option on the support line.",
			Tags:     []string{"critical", "outage", "erp-down"},
			RiskLevel: domain.RiskHigh,
			EscalationRequired: true,
		},
		{
			ID: "g_delete_tx", ClientID: domain.ClientGlobal,
			Product: "General", Module: "Order Entry",
			Question: "Can I delete a transaction?",
			Answer:   "Deleting a transaction is a high-risk action; please open a support ticket so the team can advise on a safe reversal.",
			Tags:     []string{"high-risk", "transaction", "delete"},
			RiskLevel: domain.RiskHigh,
			EscalationRequired: true,
		},
		{
			ID: "alpha_oe", ClientID: "client_alpha",
			Product: "Infor Distribution SX.e", Module: "Order Entry",
			Question: "What is our custom order entry workflow?",
			Answer:   "Client Alpha runs a customized order entry workflow with an added credit-check step between capture and release.",
			Tags:     []string{"client_alpha", "order-entry", "custom"},
			RiskLevel: domain.RiskLow,
		},
		{
			ID: "beta_sku", ClientID: "client_beta",
			Product: "Infor CloudSuite Distribution", Module: "Inventory",
			Question: "What is our private SKU pattern?",
			Answer:   "Client Beta uses a private SKU naming pattern that encodes warehouse, product family, and supplier.",
			Tags:     []string{"client_beta", "sku", "private"},
			RiskLevel: domain.RiskLow,
		},
	}
}

func TestLexicalSearch_FindsCSDRecord(t *testing.T) {
	idx := search.BuildInMemoryIndex(fixtureCorpus())
	hits, err := idx.Search("Do you support Infor CloudSuite Distribution?", domain.ClientGlobal, 5)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one hit")
	}
	if hits[0].Record.ID != "g_csd" {
		t.Fatalf("expected top hit g_csd, got %s", hits[0].Record.ID)
	}
	if hits[0].Score <= 0 || hits[0].Score > 1 {
		t.Fatalf("score out of [0,1]: %f", hits[0].Score)
	}
}

func TestLexicalSearch_ClientIsolation_AlphaCannotSeeBeta(t *testing.T) {
	idx := search.BuildInMemoryIndex(fixtureCorpus())
	// Query specifically the wording of the beta-only record.
	hits, err := idx.Search("What is our private SKU pattern?", "client_alpha", 5)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	for _, h := range hits {
		if h.Record.ID == "beta_sku" {
			t.Fatalf("client_alpha must not see client_beta record")
		}
		if h.Record.ClientID != domain.ClientGlobal && h.Record.ClientID != "client_alpha" {
			t.Fatalf("client_alpha leaked a non-global, non-alpha record: %s (client=%s)", h.Record.ID, h.Record.ClientID)
		}
	}
}

func TestLexicalSearch_ClientSpecificQuery_SeesOwnPlusGlobal(t *testing.T) {
	idx := search.BuildInMemoryIndex(fixtureCorpus())
	hits, err := idx.Search("custom order entry workflow", "client_alpha", 5)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("expected at least one hit for client_alpha")
	}
	if hits[0].Record.ID != "alpha_oe" {
		t.Fatalf("expected alpha_oe top hit, got %s", hits[0].Record.ID)
	}
}

func TestLexicalSearch_GlobalScopeExcludesClientRecords(t *testing.T) {
	idx := search.BuildInMemoryIndex(fixtureCorpus())
	hits, err := idx.Search("custom order entry workflow", domain.ClientGlobal, 10)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	for _, h := range hits {
		if !h.Record.IsGlobal() {
			t.Fatalf("global query leaked client record %s (client=%s)", h.Record.ID, h.Record.ClientID)
		}
	}
}

func TestLexicalSearch_UnrelatedQuery_LowScore(t *testing.T) {
	idx := search.BuildInMemoryIndex(fixtureCorpus())
	hits, _ := idx.Search("the weather in tokyo", domain.ClientGlobal, 5)
	if len(hits) > 0 && hits[0].Score >= 0.55 {
		t.Fatalf("expected weak match for unrelated query, got top score %f for %s", hits[0].Score, hits[0].Record.ID)
	}
}

// sanity-check that BuildInMemoryIndex did the right thing.
func TestLexicalSearch_NonEmptyCorpusIsLoaded(t *testing.T) {
	idx := search.BuildInMemoryIndex(fixtureCorpus())
	hits, _ := idx.Search("infor", domain.ClientGlobal, 5)
	if len(hits) == 0 {
		t.Fatal("expected hits for the literal token 'infor'")
	}
	// Just verify scoring returned a normalized value, not NaN/inf etc.
	top := hits[0]
	if !(top.Score >= 0 && top.Score <= 1) {
		t.Fatalf("score out of range: %v", top.Score)
	}
}
