package domain

import (
	"strings"
	"testing"
)

func TestParseOnFinishConfigJSON(t *testing.T) {
	t.Run("command defaults", func(t *testing.T) {
		config, err := ParseOnFinishConfigJSON(`{"type":"command","command":{"program":"echo","args":["ok"]}}`)
		if err != nil {
			t.Fatalf("ParseOnFinishConfigJSON() error = %v", err)
		}

		if config.Type != OnFinishSinkTypeCommand {
			t.Fatalf("Type = %q, want %q", config.Type, OnFinishSinkTypeCommand)
		}
		if config.TimeoutMS != DefaultOnFinishCommandTimeoutMS {
			t.Fatalf("TimeoutMS = %d, want %d", config.TimeoutMS, DefaultOnFinishCommandTimeoutMS)
		}
		if config.RetryCount != DefaultOnFinishRetryCount {
			t.Fatalf("RetryCount = %d, want %d", config.RetryCount, DefaultOnFinishRetryCount)
		}
		if config.RetryBackoffMS != DefaultOnFinishRetryBackoffMS {
			t.Fatalf("RetryBackoffMS = %d, want %d", config.RetryBackoffMS, DefaultOnFinishRetryBackoffMS)
		}
	})

	t.Run("http validation", func(t *testing.T) {
		config, err := ParseOnFinishConfigJSON(`{"type":"http","http":{"url":"http://127.0.0.1:8080/hooks","headers":{"Authorization":"Bearer token"}}}`)
		if err != nil {
			t.Fatalf("ParseOnFinishConfigJSON() error = %v", err)
		}

		if config.TimeoutMS != DefaultOnFinishHTTPTimeoutMS {
			t.Fatalf("TimeoutMS = %d, want %d", config.TimeoutMS, DefaultOnFinishHTTPTimeoutMS)
		}
		if config.HTTP == nil || config.HTTP.Headers["Authorization"] != "Bearer token" {
			t.Fatalf("HTTP headers = %#v, want Authorization header", config.HTTP)
		}
	})
}

func TestParseOnFinishConfigJSONRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "empty",
			raw:  "",
			want: "must not be empty",
		},
		{
			name: "missing command config",
			raw:  `{"type":"command"}`,
			want: "command sink config is required",
		},
		{
			name: "non loopback http target",
			raw:  `{"type":"http","http":{"url":"https://example.com/hooks"}}`,
			want: "must be loopback",
		},
		{
			name: "negative retry count",
			raw:  `{"type":"command","retry_count":-1,"command":{"program":"echo"}}`,
			want: "retry_count must be >= 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseOnFinishConfigJSON(tt.raw)
			if err == nil {
				t.Fatal("ParseOnFinishConfigJSON() error = nil, want error")
			}
			if got := err.Error(); got == "" || !strings.Contains(got, tt.want) {
				t.Fatalf("ParseOnFinishConfigJSON() error = %q, want substring %q", got, tt.want)
			}
		})
	}
}

func TestMarshalOnFinishConfigJSONNil(t *testing.T) {
	value, err := MarshalOnFinishConfigJSON(nil)
	if err != nil {
		t.Fatalf("MarshalOnFinishConfigJSON() error = %v", err)
	}
	if value != nil {
		t.Fatalf("MarshalOnFinishConfigJSON() = %v, want nil", value)
	}
}
