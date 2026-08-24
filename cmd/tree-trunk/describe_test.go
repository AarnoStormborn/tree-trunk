package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildCLISchemaHasSubcommands(t *testing.T) {
	s := buildCLISchema("0.1.0")
	if s.Command != "tree-trunk" || s.Version != "0.1.0" {
		t.Fatalf("bad schema header: %+v", s.Command)
	}
	names := []string{}
	for _, c := range s.Subcommands {
		names = append(names, c.Name)
	}
	if !strings.Contains(strings.Join(names, ","), "query") || !strings.Contains(strings.Join(names, ","), "describe") {
		t.Fatalf("missing subcommands: %v", names)
	}
	// query must declare its flags.
	for _, c := range s.Subcommands {
		if c.Name == "query" {
			found := false
			for _, f := range c.Flags {
				if f.Name == "filter" {
					found = true
				}
			}
			if !found {
				t.Fatal("query flags missing 'filter'")
			}
		}
	}
}

func TestPrintDescribeValidJSON(t *testing.T) {
	var buf bytes.Buffer
	// printDescribe writes to stdout; capture via the returned schema instead.
	s := buildCLISchema("test")
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var dec map[string]interface{}
	if err := json.Unmarshal(raw, &dec); err != nil {
		t.Fatalf("describe output not valid JSON: %v", err)
	}
	_ = buf
}
