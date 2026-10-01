package config

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// knownKeys are the only settings a .env file may contain. Unknown keys are
// rejected so a typo is reported instead of silently ignored.
var knownKeys = map[string]bool{
	envAddr: true,
}

// ParseDotenv reads KEY=VALUE lines. Blank lines and lines starting with '#'
// are ignored, whitespace around keys and values is trimmed, and a value may be
// wrapped in one pair of matching single or double quotes. Values are literal:
// nothing is expanded or evaluated. A repeated key keeps its last value.
func ParseDotenv(r io.Reader) (map[string]string, error) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		key = strings.TrimSpace(key)
		if !ok || !knownKeys[key] {
			return nil, fmt.Errorf("invalid configuration key at line %d", line)
		}
		values[key] = unquote(strings.TrimSpace(value))
	}
	return values, scanner.Err()
}

// readDotenvFile parses the file at path. A missing file is an empty result.
func readDotenvFile(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ParseDotenv(file)
}

func unquote(value string) string {
	if len(value) >= 2 {
		if first, last := value[0], value[len(value)-1]; first == last && (first == '"' || first == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
