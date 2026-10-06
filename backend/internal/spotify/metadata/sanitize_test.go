package metadata

import (
	"strings"
	"testing"
)

func TestSanitizePreservesDomainAndRemovesSecrets(t *testing.T) {
	raw := []byte(`{"id":"abc","unknown":{"zero":0,"false":false,"null":null,"access_token":"private","nested":[{"cookie":"private","value":2}]}}`)
	got, err := Sanitize(raw, CatalogLimit)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	for _, want := range []string{`"zero":0`, `"false":false`, `"null":null`, `"value":2`, `"unknown"`} {
		if !strings.Contains(text, want) {
			t.Fatal("domain fact lost")
		}
	}
	if strings.Contains(text, "private") {
		t.Fatal("secret retained")
	}
}
func TestSanitizeRejectsEnvelopesAndBounds(t *testing.T) {
	for _, raw := range []string{`[]`, `null`, `{} {}`, `{"headers":{}}`, strings.Repeat(" ", 20)} {
		if _, err := Sanitize([]byte(raw), 16); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}
