.PHONY: all build gen test bdd test-all ontology clean start-pdf start-pdf-binary docker

VENV_PATH := .venv
VENV_PYTHON        := $(VENV_PATH)/bin/python3
BEHAVE              := $(VENV_PATH)/bin/behave

# The ontology artefacts that server.go embeds with //go:embed. go:embed cannot
# read from the parent directory, so they are generated in ../docs and copied here.
ONTOLOGY_DIR       := docs/semantic-ontology/linkml/output
ONTOLOGY_ARTEFACTS := $(ONTOLOGY_DIR)/linkml.yaml.context.jsonld \
                      $(ONTOLOGY_DIR)/linkml.yaml.shacl.merged.ttl

# Default: ontology + build
all: docs build

# A file target, not a phony one: generation runs once and is then skipped, and
# every target that needs the artefacts (`test`, `build`, `docker`) names them as
# a prerequisite. go:embed fails in a fresh checkout without them.
$(ONTOLOGY_ARTEFACTS):
	$(MAKE) -C ../docs/semantic-ontology/linkml/ all
	rm -rf docs
	mkdir -p $(ONTOLOGY_DIR)
	cp -r ../$(ONTOLOGY_DIR)/. $(ONTOLOGY_DIR)/

# Only the ontology output is copied, not ../docs as a whole. That tree is over
# 150 MB (manuals, PDFs, the LinkML venv); pdf-core needs two files of 18 KB
# together. A full copy makes `make test` take minutes.
#
# Forces regeneration after a change to the ontology.
docs:
	rm -f $(ONTOLOGY_ARTEFACTS)
	$(MAKE) $(ONTOLOGY_ARTEFACTS)

# Start pdf-core server (for development)
start-pdf: all
	air

start-pdf-binary:
	./pdf-core

# ── Tests ────────────────────────────────────────────────────────────
# Unit + integration tests (fast, no server required)
test: $(ONTOLOGY_ARTEFACTS)
	go test ./...

# BDD tests (starts a real server, requires Python + behave)
bdd: $(ONTOLOGY_ARTEFACTS)
	$(BEHAVE) features/

# Both suites
test-all: test bdd

# ── Go build ─────────────────────────────────────────────────────────
build: $(ONTOLOGY_ARTEFACTS)
	go build -o pdf-core .

# ── Docker ───────────────────────────────────────────────────────────
docker: docs
	docker build -t dcspdfcore .

# ── Housekeeping ─────────────────────────────────────────────────────
clean:
	rm -rf pdf-core docs/
