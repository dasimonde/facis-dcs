PYTHON      ?= python3
VENV_PATH   ?= .venv
OUTPUT_PATH ?= output
SCHEMA      ?= linkml.yaml

VENV_PYTHON := $(VENV_PATH)/bin/python
VENV_PIP    := $(VENV_PATH)/bin/pip
# Marks the venv as complete. Not bin/activate: that file exists as soon as the
# venv is created, before the packages are in. If pip then fails, make would treat
# the venv as complete and the generators would stay missing.
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

# The outputs depend on their source, not on the venv: they are committed, and a
# plain build must not install the LinkML toolchain. The recipe fetches the venv
# only when it regenerates.
#
# Output goes to a temporary file first. A redirect straight to the target would
# empty the committed file before the generator runs, and a failure would leave a
# 0-byte file.
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
