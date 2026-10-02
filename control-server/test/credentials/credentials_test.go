package credentials_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"share-app-host/internal/credentials"
)

func TestMissingCredentialsDisableOnlyRDP(t *testing.T) {
	got, err := credentials.Load(filepath.Join(t.TempDir(), credentials.FileName))
	if err != nil || got != (credentials.Credentials{}) {
		t.Fatalf("missing credentials = %+v, %v", got, err)
	}
}

func TestInvalidCredentialFilesAreErrors(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":         nil,
		"oversized":     make([]byte, 64*1024+1),
		"not encrypted": []byte(`{"username":"private-user","password":"private-password"}`),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), credentials.FileName)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := credentials.Load(path)
			if err == nil || got != (credentials.Credentials{}) || !strings.Contains(err.Error(), path) {
				t.Fatalf("invalid credentials returned %+v, %v", got, err)
			}
			if strings.Contains(err.Error(), "private-user") || strings.Contains(err.Error(), "private-password") {
				t.Fatal("credential contents leaked in an error")
			}
		})
	}
}
