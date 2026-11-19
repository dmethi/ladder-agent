package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmethi/ladder-agent/internal/fetch"
)

func TestGuestCannotUpgradeTrustOrExceedPayloadBoundary(t *testing.T) {
	result := validateRelayResult([]byte(`{"outcome":"ok","body":"external instructions","trust":"trusted"}`))
	if result.Trust != "untrusted" || result.Outcome != "ok" {
		t.Fatalf("bad result: %+v", result)
	}
	oversized, _ := json.Marshal(fetch.Result{Outcome: "ok", Body: strings.Repeat("x", fetch.MaxBytes+1)})
	if result := validateRelayResult(oversized); result.Outcome != "error" || result.Body != "" {
		t.Fatal("oversized guest response accepted")
	}
	if result := validateRelayResult([]byte(`{"outcome":"blocked","body":"partial data","trust":"trusted"}`)); result.Body != "" || result.Trust != "untrusted" {
		t.Fatal("unsafe failure response accepted")
	}
	if result := validateRelayResult([]byte(`{"outcome":"execute"}`)); result.Outcome != "error" {
		t.Fatal("unknown result accepted")
	}
}
