package cmd

import (
	"testing"

	"github.com/jeehoon/graylog-cli/pkg/graylog/client"
)

func TestValidateOutput(t *testing.T) {
	for _, output := range []string{client.OutputCompact, client.OutputPretty, client.OutputJSON, client.OutputNDJSON} {
		if err := validateOutput(output); err != nil {
			t.Fatalf("validateOutput(%q) error = %v", output, err)
		}
	}

	if err := validateOutput("xml"); err == nil {
		t.Fatal("validateOutput(xml) error = nil, want error")
	}
}

func TestNewSearchRequestRejectsInvalidLimit(t *testing.T) {
	oldLimit := Limit
	Limit = 0
	defer func() {
		Limit = oldLimit
	}()

	if _, _, err := newSearchRequest(nil); err == nil {
		t.Fatal("newSearchRequest() error = nil, want invalid limit error")
	}
}

func TestBuildSearchQuery(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		application string
		part        string
		want        string
	}{
		{
			name:        "application replaces wildcard",
			query:       "*",
			application: "api",
			want:        "application:api",
		},
		{
			name:  "no filters keeps query",
			query: "level:3",
			want:  "level:3",
		},
		{
			name:        "application and part append to query",
			query:       "level:3",
			application: "api",
			part:        "worker",
			want:        "(level:3) AND application:api AND part:worker",
		},
		{
			name:        "quotes values with spaces",
			query:       "*",
			application: "my app",
			part:        "frontend part",
			want:        `application:"my app" AND part:"frontend part"`,
		},
		{
			name:        "escapes quotes in values",
			query:       "message:error",
			application: `api "blue"`,
			want:        `(message:error) AND application:"api \"blue\""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSearchQuery(tt.query, tt.application, tt.part)
			if got != tt.want {
				t.Fatalf("buildSearchQuery() = %q, want %q", got, tt.want)
			}
		})
	}
}
