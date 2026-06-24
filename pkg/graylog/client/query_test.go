package client

import "testing"

func TestAppendSearchMessageWithFields(t *testing.T) {
	query := NewSearchQuery("query-id")

	err := query.AppendSearchMessageWithFields("message-id", 5, 10, "timestamp:desc", []string{"timestamp", "message"})
	if err != nil {
		t.Fatalf("AppendSearchMessageWithFields() error = %v", err)
	}

	if len(query.SearchTypes) != 1 {
		t.Fatalf("search type count = %d, want 1", len(query.SearchTypes))
	}

	searchType, ok := query.SearchTypes[0].(*SearchTypeMessage)
	if !ok {
		t.Fatalf("search type = %T, want *SearchTypeMessage", query.SearchTypes[0])
	}
	if searchType.Limit != 5 || searchType.Offset != 10 {
		t.Fatalf("limit/offset = %d/%d, want 5/10", searchType.Limit, searchType.Offset)
	}
	if len(searchType.Sort) != 1 || searchType.Sort[0].Field != "timestamp" || searchType.Sort[0].Order != "DESC" {
		t.Fatalf("sort = %#v, want timestamp DESC", searchType.Sort)
	}
}

func TestAppendSearchMessageRejectsInvalidSort(t *testing.T) {
	query := NewSearchQuery("query-id")

	if err := query.AppendSearchMessage("message-id", 5, 0, "timestamp"); err == nil {
		t.Fatal("AppendSearchMessage() error = nil, want invalid sort error")
	}
	if err := query.AppendSearchMessage("message-id", 5, 0, "timestamp:sideways"); err == nil {
		t.Fatal("AppendSearchMessage() error = nil, want invalid sort order error")
	}
}
