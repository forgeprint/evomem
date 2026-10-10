package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// authed builds a request with a bearer token, or without one when it is
// empty.
func authed(t *testing.T, method, path, bearer string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	return r
}

// del sends a DELETE.
func del(t *testing.T, h http.Handler, path, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	return do(h, authed(t, http.MethodDelete, path, bearer))
}

// The point of the whole table: a device token works like the
// environment's, and revoking it costs only that device (ADR-0030).
func TestADeviceTokenOpensTheSameDoors(t *testing.T) {
	h, db := newServer(t, panelConfig())

	w := post(t, h, "/tokens", `{"name":"phone"}`, token)
	if w.Code != http.StatusCreated {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var made struct {
		Token  map[string]any `json:"token"`
		Secret string         `json:"secret_shown_once"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &made); err != nil {
		t.Fatal(err)
	}
	if made.Secret == "" {
		t.Fatal("no secret came back; there is no second chance to get it")
	}

	// It opens an ordinary read.
	if got := do(h, authed(t, "GET", "/notes", made.Secret)); got.Code != http.StatusOK {
		t.Fatalf("a device token was refused: %d %s", got.Code, got.Body)
	}

	id, _ := made.Token["id"].(string)
	if w := del(t, h, "/tokens/"+id, token); w.Code != http.StatusNoContent {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got := do(h, authed(t, "GET", "/notes", made.Secret)); got.Code != http.StatusUnauthorized {
		t.Fatal("a revoked token still works")
	}

	// And the environment's token is untouched.
	if got := do(h, authed(t, "GET", "/notes", token)); got.Code != http.StatusOK {
		t.Fatal("revoking a device took the environment's token with it")
	}
	_ = db
}

func TestTheSecretIsNotInTheListing(t *testing.T) {
	h, _ := newServer(t, panelConfig())

	w := post(t, h, "/tokens", `{"name":"browser"}`, token)
	var made struct {
		Secret string `json:"secret_shown_once"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &made); err != nil {
		t.Fatal(err)
	}

	listed := do(h, authed(t, "GET", "/tokens", token))
	if listed.Code != http.StatusOK {
		t.Fatalf("%d %s", listed.Code, listed.Body)
	}
	// The reply that created it is the only copy. A listing that carried
	// it would make the store worth stealing.
	if strings.Contains(listed.Body.String(), made.Secret) {
		t.Fatal("the listing hands the secret back")
	}
	if !strings.Contains(listed.Body.String(), "browser") {
		t.Fatalf("the listing does not name the token: %s", listed.Body)
	}
}

func TestTokensNeedTheToken(t *testing.T) {
	h, _ := newServer(t, panelConfig())

	for _, bearer := range []string{"", "not-the-token"} {
		if w := do(h, authed(t, "GET", "/tokens", bearer)); w.Code != http.StatusUnauthorized {
			t.Errorf("listing with %q answered %d", bearer, w.Code)
		}
		if w := post(t, h, "/tokens", `{"name":"x"}`, bearer); w.Code != http.StatusUnauthorized {
			t.Errorf("minting with %q answered %d", bearer, w.Code)
		}
	}
}

func TestATokenNeedsAName(t *testing.T) {
	h, _ := newServer(t, panelConfig())
	if w := post(t, h, "/tokens", `{"name":"  "}`, token); w.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}

func TestRevokingWhatIsNotThere(t *testing.T) {
	h, _ := newServer(t, panelConfig())
	if w := del(t, h, "/tokens/01M4D3H3HNMFM69N4MHNAYBZ1X", token); w.Code != http.StatusNotFound {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := del(t, h, "/tokens/not-a-ulid", token); w.Code != http.StatusBadRequest {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
}
