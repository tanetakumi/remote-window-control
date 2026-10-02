package credentials_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"share-app-host/internal/config"
	"share-app-host/internal/credentials"
)

func TestPowerShellScriptProducesCredentialsTheHostCanRead(t *testing.T) {
	script, err := filepath.Abs("../../../scripts/create-rdp-credentials.ps1")
	if err != nil {
		t.Fatal(err)
	}
	for name, customOutput := range map[string]bool{"default output": false, "custom output": true} {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			path := filepath.Join(base, "ShareApp", credentials.FileName)
			if customOutput {
				path = filepath.Join(base, "custom", "nested", credentials.FileName)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			// Test values cover UTF-8, quotes, backslashes and password whitespace.
			command := `$ErrorActionPreference = 'Stop'
		$secret = ConvertTo-SecureString ' p@ss"\word 日本語 ' -AsPlainText -Force
		$credential = [System.Management.Automation.PSCredential]::new('TEST\利用者', $secret)
		& $env:SHARE_APP_TEST_SCRIPT -Credential $credential`
			if customOutput {
				command += " -OutputPath $env:SHARE_APP_TEST_OUTPUT"
			}
			cmdEnv := append(os.Environ(), "LOCALAPPDATA="+base, "SHARE_APP_TEST_SCRIPT="+script, "SHARE_APP_TEST_OUTPUT="+path)
			cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command)
			cmd.Env = cmdEnv
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("credential script failed: %v: %s", err, output)
			}
			want := credentials.Credentials{Username: `TEST\利用者`, Password: ` p@ss"\word 日本語 `}
			got, err := credentials.Load(path)
			if err != nil || got != want {
				t.Fatalf("credential round trip failed: %v", err)
			}
			encrypted, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encrypted, []byte(want.Username)) || bytes.Contains(encrypted, []byte(want.Password)) {
				t.Fatal("the credential file contains plaintext credentials")
			}
			cfg, err := config.LoadFrom(config.Env{ExeDir: t.TempDir(), DataDir: filepath.Dir(path)})
			if err != nil || cfg.RDPUsername != want.Username || cfg.RDPPassword != want.Password {
				t.Fatalf("host did not load generated credentials: %v", err)
			}
			// The script must not silently replace an existing credential file.
			cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command)
			cmd.Env = cmdEnv
			if err := cmd.Run(); err == nil {
				t.Fatal("script replaced an existing file without -Force")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(encrypted, after) {
				t.Fatal("rejected script invocation changed the file")
			}
			encrypted[len(encrypted)-1] ^= 0xff
			if err := os.WriteFile(path, encrypted, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := credentials.Load(path); err == nil {
				t.Fatal("tampered credential file accepted")
			}
			cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", command+" -Force")
			cmd.Env = cmdEnv
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("credential replacement failed: %v: %s", err, output)
			}
			if got, err := credentials.Load(path); err != nil || got != want {
				t.Fatalf("replacement credentials unreadable: %v", err)
			}
		})
	}
}

func protectPayload(t *testing.T, payload []byte) []byte {
	t.Helper()
	in := windows.DataBlob{Size: uint32(len(payload)), Data: &payload[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		t.Fatal(err)
	}
	runtime.KeepAlive(payload)
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, int(out.Size))...)
}

func TestInvalidDecryptedPayloadsAreRejectedWithoutLeakingSecrets(t *testing.T) {
	for _, payload := range []string{
		`null`,
		`{"username":"","password":"private-password"}`,
		`{"username":"private-user","password":""}`,
		`{"username":"private-user","password":"private-password","private-extra":1}`,
		`{"username":"private-user","password":"private-password"} {}`,
		`{"username":"private-user","password":`,
	} {
		path := filepath.Join(t.TempDir(), credentials.FileName)
		if err := os.WriteFile(path, protectPayload(t, []byte(payload)), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := credentials.Load(path)
		if err == nil || got != (credentials.Credentials{}) {
			t.Fatal("invalid decrypted credentials accepted")
		}
		if strings.Contains(err.Error(), "private-") {
			t.Fatal("credential payload leaked in an error")
		}
	}
}
