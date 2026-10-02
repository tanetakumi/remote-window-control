// Package credentials reads optional RDP credentials protected by Windows DPAPI.
// It has no API for creating or updating credentials; the standalone PowerShell
// script generates the encrypted file outside the host.
package credentials

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const FileName = "rdp-credentials.bin"
const maxFileBytes = 64 * 1024

// Credentials are only passed to the loopback RDP client, never to HTTP clients.
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Load returns empty credentials when the file is absent. An existing but
// unreadable, corrupt or undecryptable file is an error, not an absent file.
func Load(path string) (Credentials, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return Credentials{}, nil
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("%s: cannot read RDP credentials: %w", path, err)
	}
	defer file.Close()
	encrypted, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return Credentials{}, fmt.Errorf("%s: cannot read RDP credentials: %w", path, err)
	}
	if len(encrypted) == 0 || len(encrypted) > maxFileBytes {
		return Credentials{}, fmt.Errorf("%s: invalid RDP credential file size", path)
	}
	plain, err := unprotect(encrypted)
	if err != nil {
		return Credentials{}, fmt.Errorf("%s: cannot decrypt RDP credentials; generate the file on this PC as the user running Share App: %w", path, err)
	}
	defer clear(plain)
	var result Credentials
	decoder := json.NewDecoder(bytes.NewReader(plain))
	decoder.DisallowUnknownFields()
	if !utf8.Valid(plain) || decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF ||
		strings.TrimSpace(result.Username) == "" || result.Password == "" {
		// Decoder errors can include plaintext field names. Do not log the payload.
		return Credentials{}, fmt.Errorf("%s: invalid RDP credential payload", path)
	}
	return result, nil
}
