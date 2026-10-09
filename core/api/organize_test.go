package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/forgeprint/evomem/shared/database"
)

// organizeConfig is the panel with a grouping function wired in.
func organizeConfig(fn OrganizeFunc) Config {
	cfg := panelConfig()
	cfg.Organize = fn
	return cfg
}

func TestOrganizeNeedsTheToken(t *testing.T) {
	h, _ := newServer(t, organizeConfig(func(context.Context, database.SecretKey, string, bool) (OrganizeResult, error) {
		t.Fatal("an unauthenticated request reached the model")
		return OrganizeResult{}, nil
	}))

	if w := post(t, h, "/organize", "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := post(t, h, "/organize", "", "not-the-token"); w.Code != http.StatusUnauthorized {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestOrganizeReportsWhatTheRunDid(t *testing.T) {
	var sawProject string
	var sawDryRun bool
	h, _ := newServer(t, organizeConfig(func(_ context.Context, _ database.SecretKey, project string, dryRun bool) (OrganizeResult, error) {
		sawProject, sawDryRun = project, dryRun
		return OrganizeResult{Offered: 7, Created: 2, Grouped: 5, Invented: 1, Dropped: 1}, nil
	}))

	w := post(t, h, "/organize?project=evomem", "", token)
	if w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if sawProject != "evomem" {
		t.Fatalf("project: %q", sawProject)
	}
	if sawDryRun {
		t.Fatal("a run without dry_run=1 should write")
	}

	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for field, want := range map[string]float64{
		"offered": 7, "created": 2, "grouped": 5, "invented": 1, "dropped": 1,
	} {
		if got[field] != want {
			t.Errorf("%s: got %v, want %v", field, got[field], want)
		}
	}
}

func TestOrganizeDryRunIsPassedThrough(t *testing.T) {
	var sawDryRun bool
	h, _ := newServer(t, organizeConfig(func(_ context.Context, _ database.SecretKey, _ string, dryRun bool) (OrganizeResult, error) {
		sawDryRun = dryRun
		return OrganizeResult{}, nil
	}))

	if w := post(t, h, "/organize?dry_run=1", "", token); w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !sawDryRun {
		t.Fatal("dry_run=1 did not reach the run")
	}
}

// No model connected is the switch being off, not a failure of this server,
// and the panel has to be able to tell those apart (ADR-0027).
func TestOrganizeWithNoModelSaysNothingWasSent(t *testing.T) {
	h, _ := newServer(t, organizeConfig(func(context.Context, database.SecretKey, string, bool) (OrganizeResult, error) {
		return OrganizeResult{}, ErrNoModel
	}))

	w := post(t, h, "/organize", "", token)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "nothing was sent anywhere") {
		t.Fatalf("the body does not say nothing left the machine: %q", w.Body)
	}
}

// The model's own words reach the panel: "incorrect api key" and "model not
// found" are what actually goes wrong, and the person at the panel can fix
// both only if they are told which.
func TestOrganizeCarriesTheModelsReason(t *testing.T) {
	h, _ := newServer(t, organizeConfig(func(context.Context, database.SecretKey, string, bool) (OrganizeResult, error) {
		return OrganizeResult{}, errors.New("organize: the model answered 401 Unauthorized: incorrect api key")
	}))

	w := post(t, h, "/organize", "", token)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !strings.Contains(w.Body.String(), "incorrect api key") {
		t.Fatalf("the model's reason did not reach the panel: %q", w.Body)
	}
}

// Without the key the stored model key cannot be unsealed, and that is a
// server configuration problem the person at the panel cannot fix there.
func TestOrganizeWithoutASecretKeyIsUnavailable(t *testing.T) {
	cfg := organizeConfig(func(context.Context, database.SecretKey, string, bool) (OrganizeResult, error) {
		t.Fatal("ran without a usable key")
		return OrganizeResult{}, nil
	})
	cfg.SecretKey = ""
	h, _ := newServer(t, cfg)

	if w := post(t, h, "/organize", "", token); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
