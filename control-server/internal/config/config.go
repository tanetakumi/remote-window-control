package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct{ ListenAddr, ClientDir, BaseDir string }

func Load() (Config, error) {
	base := resolveBaseDir()
	values, err := readConfig(filepath.Join(base, ".env"))
	if err != nil {
		return Config{}, err
	}
	value := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		if v := values[key]; v != "" {
			return v
		}
		return fallback
	}
	client := value("SHARE_APP_CLIENT_DIR", resolveClientDir(base))
	if !filepath.IsAbs(client) {
		client = filepath.Join(base, client)
	}
	return Config{ListenAddr: value("SHARE_APP_ADDR", "127.0.0.1:8443"), ClientDir: client, BaseDir: base}, nil
}

// Configuration is data, never shell code. Environment variables override the file.
func readConfig(path string) (map[string]string, error) {
	result := make(map[string]string)
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || (key != "SHARE_APP_ADDR" && key != "SHARE_APP_CLIENT_DIR") {
			return nil, fmt.Errorf("invalid configuration key at line %d", line)
		}
		if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
		result[key] = value
	}
	return result, scanner.Err()
}
func resolveBaseDir() string {
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	for _, entry := range []string{"web", "CaptureProbe"} {
		if info, err := os.Stat(filepath.Join(exeDir, entry)); err == nil && info.IsDir() {
			return exeDir
		}
	}
	cwd, err := os.Getwd()
	if err == nil {
		for dir := cwd; ; dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, "control-server", "go.mod")); err == nil {
				return dir
			}
			if filepath.Dir(dir) == dir {
				break
			}
		}
	}
	return exeDir
}
func resolveClientDir(base string) string {
	web := filepath.Join(base, "web")
	if info, err := os.Stat(web); err == nil && info.IsDir() {
		return web
	}
	return filepath.Join(base, "web-ui", "dist")
}
