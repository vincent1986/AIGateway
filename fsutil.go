package main

import (
	"encoding/json"
	"os"
)

func mkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func writeJSONFile(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
