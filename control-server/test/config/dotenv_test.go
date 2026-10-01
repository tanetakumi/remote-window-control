package config_test

import (
	"reflect"
	"strings"
	"testing"

	"share-app-host/internal/config"
)

func TestParseDotenv(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{"empty", "", map[string]string{}},
		{"comments and blank lines", "# a comment\n\n   \n  # indented comment\n", map[string]string{}},
		{"plain values", "SHARE_APP_ADDR=127.0.0.1:9000\nSHARE_APP_CLIENT_DIR=web\n",
			map[string]string{"SHARE_APP_ADDR": "127.0.0.1:9000", "SHARE_APP_CLIENT_DIR": "web"}},
		{"whitespace is trimmed", "  SHARE_APP_ADDR  =  :8443  \n", map[string]string{"SHARE_APP_ADDR": ":8443"}},
		{"double quotes are stripped", `SHARE_APP_ADDR="127.0.0.1:1"`, map[string]string{"SHARE_APP_ADDR": "127.0.0.1:1"}},
		{"single quotes are stripped", `SHARE_APP_ADDR='127.0.0.1:1'`, map[string]string{"SHARE_APP_ADDR": "127.0.0.1:1"}},
		{"mismatched quotes are kept", `SHARE_APP_ADDR="abc'`, map[string]string{"SHARE_APP_ADDR": `"abc'`}},
		{"a lone quote is kept", `SHARE_APP_ADDR="`, map[string]string{"SHARE_APP_ADDR": `"`}},
		{"values are literal, never evaluated", "SHARE_APP_CLIENT_DIR='literal-$(command) `x` $HOME'\n",
			map[string]string{"SHARE_APP_CLIENT_DIR": "literal-$(command) `x` $HOME"}},
		{"value may contain equals signs", "SHARE_APP_CLIENT_DIR=a=b\n", map[string]string{"SHARE_APP_CLIENT_DIR": "a=b"}},
		{"empty value", "SHARE_APP_ADDR=\n", map[string]string{"SHARE_APP_ADDR": ""}},
		{"the last duplicate wins", "SHARE_APP_ADDR=a\nSHARE_APP_ADDR=b\n", map[string]string{"SHARE_APP_ADDR": "b"}},
		{"CRLF line endings", "SHARE_APP_ADDR=a\r\nSHARE_APP_CLIENT_DIR=b\r\n",
			map[string]string{"SHARE_APP_ADDR": "a", "SHARE_APP_CLIENT_DIR": "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ParseDotenv(strings.NewReader(tt.input))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseDotenvRejectsInvalidLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLine string
	}{
		{"unknown key", "SHARE_APP_ADDR=a\nSHARE_APP_TYPO=b\n", "line 2"},
		{"missing equals", "SHARE_APP_ADDR\n", "line 1"},
		{"empty key", "=value\n", "line 1"},
		{"keys are case sensitive", "share_app_addr=x\n", "line 1"},
		{"shell syntax", "export SHARE_APP_ADDR=x\n", "line 1"},
		{"error line counts comments and blanks", "# c\n\nBAD\n", "line 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := config.ParseDotenv(strings.NewReader(tt.input))
			if err == nil || !strings.Contains(err.Error(), tt.wantLine) {
				t.Fatalf("error = %v, want one mentioning %q", err, tt.wantLine)
			}
		})
	}
}
