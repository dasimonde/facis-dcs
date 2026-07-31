package configattest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHashFilesSkipsUnsetAndFailsUnreadable(t *testing.T) {
	dir := t.TempDir()
	didPath := writeFile(t, dir, "did.json", `{"id":"did:web:x"}`)

	hashes, err := HashFiles(map[string]string{"did-document": didPath, "optional": ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(hashes) != 1 || len(hashes["did-document"]) != 64 {
		t.Fatalf("expected one 64-hex hash for did-document, got: %v", hashes)
	}

	if _, err := HashFiles(map[string]string{"gone": filepath.Join(dir, "missing.json")}); err == nil {
		t.Fatal("a set-but-unreadable config path must be a hard error")
	}
}

func TestVerifyPinsDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "trust.json", `{"anchors":[]}`)
	hashes, err := HashFiles(map[string]string{"trust": path})
	if err != nil {
		t.Fatal(err)
	}

	// Pin matches the genuine file.
	if err := VerifyPins(hashes, map[string]string{"trust": hashes["trust"]}); err != nil {
		t.Fatalf("genuine file must pass its pin: %v", err)
	}

	// The file is tampered after pinning: hashes recomputed at next startup
	// differ from the pin and verification must fail naming the file.
	writeFile(t, dir, "trust.json", `{"anchors":["did:web:evil"]}`)
	tampered, err := HashFiles(map[string]string{"trust": path})
	if err != nil {
		t.Fatal(err)
	}
	err = VerifyPins(tampered, map[string]string{"trust": hashes["trust"]})
	if err == nil || !strings.Contains(err.Error(), `"trust"`) {
		t.Fatalf("tampered file must fail pin verification naming the file, got: %v", err)
	}

	// A pinned file that is absent entirely must fail too.
	if err := VerifyPins(map[string]string{}, map[string]string{"trust": hashes["trust"]}); err == nil {
		t.Fatal("a pinned but absent config file must fail verification")
	}
}

func TestParsePins(t *testing.T) {
	pins, err := ParsePins(" did-document=" + strings.Repeat("ab", 32) + " , trust=" + strings.Repeat("01", 32))
	if err != nil || len(pins) != 2 {
		t.Fatalf("expected two parsed pins, got %v / %v", pins, err)
	}
	for _, bad := range []string{"did-document", "x=notahash", "x=" + strings.Repeat("g", 64), "=" + strings.Repeat("ab", 32)} {
		if _, err := ParsePins(bad); err == nil {
			t.Fatalf("malformed pin %q must be rejected", bad)
		}
	}
	empty, err := ParsePins("  ")
	if err != nil || len(empty) != 0 {
		t.Fatalf("blank pins env must parse to no pins, got %v / %v", empty, err)
	}
}
