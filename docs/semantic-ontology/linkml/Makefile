PYTHON      ?= python3
VENV_PATH   ?= .venv
OUTPUT_PATH ?= output
SCHEMA      ?= linkml.yaml

VENV_PYTHON := $(VENV_PATH)/bin/python
VENV_PIP    := $(VENV_PATH)/bin/pip
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

setup_venv: $(VENV_PATH)/bin/activate

$(VENV_PATH)/bin/activate:
	$(PYTHON) -m venv $(VENV_PATH)
	$(VENV_PIP) install --upgrade pip
	$(VENV_PIP) install linkml pyshacl

$(OUTPUT_PATH):
	mkdir -p $(OUTPUT_PATH)

$(OUTPUT_PATH)/$(SCHEMA).shacl.ttl: setup_venv $(OUTPUT_PATH) $(SCHEMA)
	$(VENV_PYTHON) $(GEN_SHACL) --non-closed $(SCHEMA) > $(OUTPUT_PATH)/$(SCHEMA).shacl.ttl

$(OUTPUT_PATH)/$(SCHEMA).context.jsonld: setup_venv $(OUTPUT_PATH) $(SCHEMA)
	$(VENV_PYTHON) $(GEN_JSONLD_CONTEXT) $(SCHEMA) > $(OUTPUT_PATH)/$(SCHEMA).context.jsonld
	$(VENV_PYTHON) -c "\
import json; \
path = '$(OUTPUT_PATH)/$(SCHEMA).context.jsonld'; \
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

$(SHAPES_MERGED): $(OUTPUT_PATH)/$(SCHEMA).shacl.ttl $(SHAPES_EXTRA)
	cat $^ > $@

GREEN := \033[0;32m
RED   := \033[0;31m
RESET := \033[0m

validate: setup_venv $(SHAPES_MERGED)
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
