package main

import (
	"strconv"
	"strings"
)

type tomlProviderTable struct {
	ID    string
	Start int
	End   int
}

func parseTomlProviderTables(content string) []tomlProviderTable {
	var tables []tomlProviderTable
	offset := 0
	lines := strings.SplitAfter(content, "\n")
	for _, line := range lines {
		raw := strings.TrimSpace(strings.TrimSuffix(line, "\n"))
		if id, ok := parseModelProviderHeader(raw); ok {
			tables = append(tables, tomlProviderTable{ID: id, Start: offset})
		}
		offset += len(line)
	}
	for i := range tables {
		if i+1 < len(tables) {
			tables[i].End = tables[i+1].Start
		} else {
			tables[i].End = len(content)
		}
	}
	return tables
}

func parseModelProviderHeader(header string) (string, bool) {
	if !strings.HasPrefix(header, "[") || !strings.HasSuffix(header, "]") || strings.HasPrefix(header, "[[") {
		return "", false
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(header, "["), "]"))
	const prefix = "model_providers."
	if !strings.HasPrefix(body, prefix) {
		return "", false
	}
	id := strings.TrimSpace(strings.TrimPrefix(body, prefix))
	if id == "" || strings.Contains(id, ".") && !strings.HasPrefix(id, `"`) {
		return "", false
	}
	if strings.HasPrefix(id, `"`) {
		unquoted, err := strconv.Unquote(id)
		if err != nil || unquoted == "" {
			return "", false
		}
		return unquoted, true
	}
	return id, true
}

func findProviderTable(content, providerID string) (tomlProviderTable, bool) {
	for _, table := range parseTomlProviderTables(content) {
		if table.ID == providerID {
			return table, true
		}
	}
	return tomlProviderTable{}, false
}

func tomlProviderIDs(content string) []string {
	tables := parseTomlProviderTables(content)
	ids := make([]string, 0, len(tables))
	for _, table := range tables {
		ids = append(ids, table.ID)
	}
	return ids
}
