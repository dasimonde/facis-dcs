.PHONY: all build gen test bdd test-all ontology clean start-pdf start-pdf-binary docker

VENV_PATH := .venv
VENV_PYTHON        := $(VENV_PATH)/bin/python3
BEHAVE              := $(VENV_PATH)/bin/behave

# Die Ontologie-Artefakte, die server.go per //go:embed einbindet. go:embed
# darf nicht aus dem Elternverzeichnis lesen, deshalb werden sie aus ../docs
# erzeugt und hierher kopiert.
ONTOLOGY_DIR       := docs/semantic-ontology/linkml/output
ONTOLOGY_ARTEFACTS := $(ONTOLOGY_DIR)/linkml.yaml.context.jsonld \
                      $(ONTOLOGY_DIR)/linkml.yaml.shacl.merged.ttl

# Default: ontology + build
all: docs build

# Ein Dateiziel, kein Phony: so laeuft die Erzeugung einmal und wird danach
# uebersprungen, und jedes Ziel, das die Artefakte braucht, kann sie einfach
# als Voraussetzung nennen. Vorher hing nur `all` und `docker` an `docs`,
# `test` und `build` nicht — beide brachen im frischen Checkout ab, weil
# go:embed die Dateien nicht fand.
$(ONTOLOGY_ARTEFACTS):
	$(MAKE) -C ../docs/semantic-ontology/linkml/ all
	rm -rf docs
	mkdir -p $(ONTOLOGY_DIR)
	cp -r ../$(ONTOLOGY_DIR)/. $(ONTOLOGY_DIR)/

# Kopiert wird nur das Ontologie-Ergebnis, nicht ../docs als Ganzes. Der Baum
# dort ist ueber 150 MB gross (Handbuecher, Screenshots, PDFs und das
# LinkML-venv); gebraucht werden zwei Dateien von zusammen 18 KB. Die
# Gesamtkopie hat `make test` auf mehrere Minuten gebracht, obwohl das Ziel
# fuer schnelle Rueckmeldung gedacht ist.
#
# Erzwingt die Neuerzeugung, wenn sich die Ontologie geaendert hat.
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
