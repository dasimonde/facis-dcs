package compiler

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	loadDevEnv()
	setupTestContext()
	if err := setupTestSigning(); err != nil {
		panic("setupTestSigning: " + err.Error())
	}
	code := m.Run()
	os.Exit(code)
}

func setupTestContext() {
	b, err := os.ReadFile("../docs/semantic-ontology/linkml/output/linkml.yaml.context.jsonld")
	if err != nil {
		panic("setupTestContext: " + err.Error())
	}
	baseURL := os.Getenv("DCS_PDF_CORE_ONTOLOGY_BASE_URL")
	if baseURL == "" {
		panic("setupTestContext: DCS_PDF_CORE_ONTOLOGY_BASE_URL must be set (check .dev.env)")
	}
	SetContextDocument(baseURL+"/ontology/dcs-pdf-core", b)
}

func loadDevEnv() {
	f, err := os.Open(".dev.env")
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if key != "" {
			_ = os.Setenv(key, value)
		}
	}
}
