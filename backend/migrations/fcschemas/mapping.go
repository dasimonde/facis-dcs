package fcschemas

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed version_mapping.yaml
//go:embed schemas/*.ttl
var bundledFiles embed.FS

type versionMappingFile struct {
	Types map[string][]versionMappingEntry `yaml:"types"`
}

type versionMappingEntry struct {
	Version  int               `yaml:"version"`
	File     string            `yaml:"file"`
	Prefixes map[string]string `yaml:"prefixes"`
}

// Bundle is one embedded schema version ready to sync to FC.
type Bundle struct {
	Type     string
	Version  int
	File     string
	Content  []byte
	Prefixes map[string]string
}

// LoadBundles reads version_mapping.yaml and embedded schema files.
func LoadBundles() ([]Bundle, error) {
	raw, err := fs.ReadFile(bundledFiles, "version_mapping.yaml")
	if err != nil {
		return nil, fmt.Errorf("read version_mapping.yaml: %w", err)
	}

	var doc versionMappingFile
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse version_mapping.yaml: %w", err)
	}
	if len(doc.Types) == 0 {
		return nil, fmt.Errorf("version_mapping.yaml: no types defined")
	}

	var bundles []Bundle
	for schemaType, entries := range doc.Types {
		schemaType = strings.TrimSpace(schemaType)
		if schemaType == "" {
			return nil, fmt.Errorf("version_mapping.yaml: empty schema type name")
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("version_mapping.yaml: type %q has no versions", schemaType)
		}

		seen := make(map[int]struct{}, len(entries))
		for _, entry := range entries {
			if entry.Version <= 0 {
				return nil, fmt.Errorf("version_mapping.yaml: %s: version must be positive (file %q)", schemaType, entry.File)
			}
			if entry.File == "" {
				return nil, fmt.Errorf("version_mapping.yaml: %s: file is required for version %d", schemaType, entry.Version)
			}
			if _, dup := seen[entry.Version]; dup {
				return nil, fmt.Errorf("version_mapping.yaml: %s: duplicate version %d", schemaType, entry.Version)
			}
			seen[entry.Version] = struct{}{}

			prefixes, err := normalizePrefixes(schemaType, entry.Version, entry.Prefixes)
			if err != nil {
				return nil, err
			}

			path := "schemas/" + entry.File
			content, err := fs.ReadFile(bundledFiles, path)
			if err != nil {
				return nil, fmt.Errorf("read schema %s: %w", path, err)
			}
			if err := validateTTLMatchesPrefixes(content, prefixes); err != nil {
				return nil, fmt.Errorf("schema %s (%s): %w", schemaType, entry.File, err)
			}

			bundles = append(bundles, Bundle{
				Type:     schemaType,
				Version:  entry.Version,
				File:     entry.File,
				Content:  content,
				Prefixes: prefixes,
			})
		}
	}

	sort.Slice(bundles, func(i, j int) bool {
		if bundles[i].Type != bundles[j].Type {
			return bundles[i].Type < bundles[j].Type
		}
		return bundles[i].Version < bundles[j].Version
	})
	return bundles, nil
}

func normalizePrefixes(schemaType string, version int, raw map[string]string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("version_mapping.yaml: %s v%d: prefixes required", schemaType, version)
	}
	out := make(map[string]string, len(raw))
	for name, iri := range raw {
		name = strings.TrimSpace(name)
		iri = strings.TrimSpace(iri)
		if name == "" || iri == "" {
			return nil, fmt.Errorf("version_mapping.yaml: %s v%d: empty prefix name or IRI", schemaType, version)
		}
		out[name] = iri
	}
	return out, nil
}

func validateTTLMatchesPrefixes(content []byte, prefixes map[string]string) error {
	text := string(content)
	for name, iri := range prefixes {
		if !strings.Contains(text, iri) {
			return fmt.Errorf("missing namespace IRI %q for prefix %q", iri, name)
		}
		if !strings.Contains(text, "@prefix "+name+":") {
			return fmt.Errorf("missing @prefix %s declaration", name)
		}
	}
	return nil
}

// MatchesRemote reports whether remote TTL belongs to this bundle.
func (b Bundle) MatchesRemote(remote []byte) bool {
	text := string(remote)
	for name, iri := range b.Prefixes {
		if !strings.Contains(text, iri) || !strings.Contains(text, "@prefix "+name+":") {
			return false
		}
	}
	return true
}
