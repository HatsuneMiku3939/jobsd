package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Format
		wantErr bool
	}{
		{name: "table", input: "table", want: FormatTable},
		{name: "json", input: "json", want: FormatJSON},
		{name: "invalid", input: "yaml", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFormat(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ParseFormat() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFormat() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ParseFormat() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrinter(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		var buffer bytes.Buffer
		printer := New(&buffer, FormatJSON)

		if err := printer.PrintJSON(map[string]string{"status": "ok"}); err != nil {
			t.Fatalf("PrintJSON() error = %v", err)
		}
		if !strings.Contains(buffer.String(), "\"status\": \"ok\"") {
			t.Fatalf("PrintJSON() output = %q", buffer.String())
		}
	})

	t.Run("table", func(t *testing.T) {
		var buffer bytes.Buffer
		printer := New(&buffer, FormatTable)

		if err := printer.PrintTable([]string{"NAME", "STATE"}, [][]string{{"demo", "ready"}}); err != nil {
			t.Fatalf("PrintTable() error = %v", err)
		}
		output := buffer.String()
		if !strings.Contains(output, "NAME") || !strings.Contains(output, "demo") {
			t.Fatalf("PrintTable() output = %q", output)
		}
	})
}
