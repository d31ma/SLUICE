package yon

import (
	"path/filepath"
	"testing"
)

func TestRequestBodyBoundaries(t *testing.T) {
	tests := []struct {
		name    string
		body    *RequestBody
		want    string
		wantErr bool
	}{
		{name: "absent"},
		{name: "default text", body: &RequestBody{Data: "hello"}, want: "hello"},
		{name: "utf8", body: &RequestBody{Encoding: "UTF-8", Data: "hello"}, want: "hello"},
		{name: "base64", body: &RequestBody{Encoding: "base64", Data: "aGVsbG8="}, want: "hello"},
		{name: "invalid base64", body: &RequestBody{Encoding: "base64", Data: "%%%"}, wantErr: true},
		{name: "unsupported", body: &RequestBody{Encoding: "gzip", Data: "hello"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := requestBody(test.body)
			if (err != nil) != test.wantErr {
				t.Fatalf("requestBody() error = %v, wantErr %t", err, test.wantErr)
			}
			if string(got) != test.want {
				t.Fatalf("requestBody() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestValidateServeConfigBoundaries(t *testing.T) {
	project := t.TempDir()
	tests := []struct {
		name   string
		config ServeConfig
	}{
		{name: "relative project", config: ServeConfig{ProjectRoot: "project", Host: "127.0.0.1", Port: 1}},
		{name: "missing project", config: ServeConfig{ProjectRoot: filepath.Join(project, "missing"), Host: "127.0.0.1", Port: 1}},
		{name: "non-loopback host", config: ServeConfig{ProjectRoot: project, Host: "0.0.0.0", Port: 1}},
		{name: "zero port", config: ServeConfig{ProjectRoot: project, Host: "127.0.0.1", Port: 0}},
		{name: "port above maximum", config: ServeConfig{ProjectRoot: project, Host: "127.0.0.1", Port: 65536}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := validateServeConfig(test.config); err == nil {
				t.Fatal("validateServeConfig() error = nil, want rejection")
			}
		})
	}

	got, err := validateServeConfig(ServeConfig{ProjectRoot: project, Host: "::1", Port: 65535})
	if err != nil {
		t.Fatalf("validateServeConfig() valid boundary error: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("validated project = %q, want absolute path", got)
	}
}
