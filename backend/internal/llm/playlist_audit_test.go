package llm

import "testing"

func TestParsePlaylistAuditResponseValidatesKnownIDsAndDeduplicates(t *testing.T) {
	result, err := ParsePlaylistAuditResponse(`{"rejected":[{"id":"christmas","constraint":"no Christmas music","reason":"Christmas title"},{"id":"christmas","constraint":"duplicate","reason":"duplicate"}]}`, map[string]struct{}{"christmas": {}, "jazz": {}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rejected) != 1 || result.Rejected[0].ID != "christmas" {
		t.Fatalf("result=%#v", result)
	}
}

func TestParsePlaylistAuditResponseRejectsUnknownIDsAndFields(t *testing.T) {
	if _, err := ParsePlaylistAuditResponse(`{"rejected":[{"id":"invented","constraint":"x","reason":"y"}]}`, map[string]struct{}{"known": {}}); err == nil {
		t.Fatal("unknown id accepted")
	}
	if _, err := ParsePlaylistAuditResponse(`{"rejected":[],"extra":true}`, map[string]struct{}{}); err == nil {
		t.Fatal("unknown field accepted")
	}
	if _, err := ParsePlaylistAuditResponse(`{}`, map[string]struct{}{}); err == nil {
		t.Fatal("missing rejected array accepted")
	}
	if _, err := ParsePlaylistAuditResponse(`{"rejected":[{"id":"known","constraint":"","reason":"missing evidence"}]}`, map[string]struct{}{"known": {}}); err == nil {
		t.Fatal("incomplete rejection accepted")
	}
}
