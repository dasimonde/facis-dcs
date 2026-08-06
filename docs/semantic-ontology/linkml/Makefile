PYTHON      ?= python3
VENV_PATH   ?= .venv
OUTPUT_PATH ?= output
SCHEMA      ?= linkml.yaml

VENV_PYTHON := $(VENV_PATH)/bin/python
VENV_PIP    := $(VENV_PATH)/bin/pip
# Fertigmeldung des venv. Nicht activate: die Datei entsteht schon beim Anlegen
# des venv, also bevor die Pakete drin sind. Bricht pip danach ab, gilt das
# venv fuer make trotzdem als fertig, und die Generatoren fehlen fuer immer.
VENV_STAMP  := $(VENV_PATH)/.install-stamp
GEN_SHACL          := $(VENV_PATH)/bin/gen-shacl
GEN_JSONLD_CONTEXT := $(VENV_PATH)/bin/gen-jsonld-context
PYSHACL            := $(VENV_PATH)/bin/pyshacl

SHAPES_EXTRA  ?= shapes-content.ttl
SHAPES_MERGED := $(OUTPUT_PATH)/$(SCHEMA).shacl.merged.ttl

# Properties that form ordered RDF lists of embedded objects (@container: @list).
LIST_PROPS := blocks content layout
# Properties that are ordered RDF lists of IRI references (@type: @id, @container: @list).
IRI_LIST_PROPS := children
# Properties that are unordered IRI-reference sets (@type: @id, @container: @set).
IRI_SET_PROPS := signatureFields

VALID_TESTS   := $(wildcard tests/valid/*.jsonld)
INVALID_TESTS := $(wildcard tests/invalid/*.jsonld)

.PHONY: all setup_venv validate clean

all: $(SHAPES_MERGED) $(OUTPUT_PATH)/$(SCHEMA).context.jsonld

setup_venv: $(VENV_STAMP)

$(VENV_STAMP):
	$(PYTHON) -m venv $(VENV_PATH)
	$(VENV_PIP) install --upgrade pip
	$(VENV_PIP) install linkml pyshacl
	touch $@

$(OUTPUT_PATH):
	mkdir -p $(OUTPUT_PATH)

# Die Erzeugnisse haengen an ihrer Quelle, nicht am venv: sie liegen im
# Repository, und wer nur baut, soll nicht die halbe LinkML-Toolchain
# installieren muessen. Das venv wird erst im Rezept geholt, also nur dann,
# wenn wirklich neu erzeugt wird.
#
# Geschrieben wird ueber eine Zwischendatei. Eine Umleitung direkt auf das Ziel
# leert die versionierte Datei, bevor der Generator ueberhaupt laeuft — ein
# Fehlschlag hinterliess so eine 0-Byte-Datei im Arbeitsverzeichnis.
$(OUTPUT_PATH)/$(SCHEMA).shacl.ttl: $(SCHEMA) | $(OUTPUT_PATH)
	$(MAKE) $(VENV_STAMP)
	$(VENV_PYTHON) $(GEN_SHACL) --non-closed $(SCHEMA) > $@.tmp
	mv $@.tmp $@

$(OUTPUT_PATH)/$(SCHEMA).context.jsonld: $(SCHEMA) | $(OUTPUT_PATH)
	$(MAKE) $(VENV_STAMP)
	$(VENV_PYTHON) $(GEN_JSONLD_CONTEXT) $(SCHEMA) > $@.tmp
	$(VENV_PYTHON) -c "\
import json; \
path = '$(OUTPUT_PATH)/$(SCHEMA).context.jsonld.tmp'; \
doc = json.load(open(path)); \
ctx = doc['@context']; \
ctx.pop('dcs', None); \
list(map(lambda p: ctx.setdefault(p, {}).__setitem__('@container', '@list'), '$(LIST_PROPS)'.split())); \
list(map(lambda p: ctx.setdefault(p, {}).update({'@type': '@id', '@container': '@list'}), '$(IRI_LIST_PROPS)'.split())); \
list(map(lambda p: ctx.setdefault(p, {}).update({'@type': '@id', '@container': '@set'}), '$(IRI_SET_PROPS)'.split())); \
list(map(lambda p: ctx.get(p, {}).pop('@type', None), ('isRoot', 'required'))); \
ctx.get('templateVersion', {}).pop('@type', None); \
(ctx.__setitem__('schema', 'https://schema.org/') if ctx.get('schema') == 'http://schema.org/' else None); \
json.dump(doc, open(path, 'w'), indent=3)"
	mv $@.tmp $@

$(SHAPES_MERGED): $(OUTPUT_PATH)/$(SCHEMA).shacl.ttl $(SHAPES_EXTRA)
	cat $^ > $@.tmp
	mv $@.tmp $@

GREEN := \033[0;32m
RED   := \033[0;31m
RESET := \033[0m

validate: $(VENV_STAMP) $(SHAPES_MERGED)
	@failed=0; \
	for f in $(VALID_TESTS); do \
	    out=$$($(VENV_PYTHON) $(PYSHACL) -s $(SHAPES_MERGED) -df json-ld $$f 2>&1); \
	    if echo "$$out" | grep -q "Conforms: True"; then \
	        printf "$(GREEN)PASS$(RESET)  $$f\n"; \
	    else \
	        printf "$(RED)FAIL$(RESET)  $$f  (expected valid)\n"; \
	        echo "$$out"; failed=1; \
	    fi; \
	done; \
	for f in $(INVALID_TESTS); do \
	    out=$$($(VENV_PYTHON) $(PYSHACL) -s $(SHAPES_MERGED) -df json-ld $$f 2>&1); \
	    if ! echo "$$out" | grep -q "Conforms: True"; then \
	        printf "$(GREEN)PASS$(RESET)  $$f  (correctly rejected)\n"; \
	        echo "$$out" | grep -E "Constraint Violation|Message:|Result Path:" | sed 's/^/      /'; \
	    else \
	        printf "$(RED)FAIL$(RESET)  $$f  (expected rejection)\n"; failed=1; \
	    fi; \
	done; \
	exit $$failed

clean:
	rm -rf $(OUTPUT_PATH)
