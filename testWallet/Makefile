VENV_DIR ?= .venv
PYTHON := $(VENV_DIR)/bin/python
PIP := $(VENV_DIR)/bin/pip
VENV_STAMP := $(VENV_DIR)/.install-stamp

.PHONY: install ensure-statuslist test sign

# Sign a contract as the test wallet+QTSP (prepare -> sign via DSS -> submit).
# The DCS holds no key; this plays the signatory (ADR-12). Example:
#   make sign DCS_URL=http://localhost:8991/api CONTRACT_DID=<did> \
#     FIELD=<field> USER=johndoe SIGNER_DID=<did> TOKEN=<jwt>
DCS_URL ?= http://localhost:8991/api
DSS_URL ?= http://localhost:18099
sign: $(VENV_STAMP)
	$(PYTHON) -m dcs_wallet.ceremony_driver \
	  --dcs-url "$(DCS_URL)" --dss-url "$(DSS_URL)" --token "$(TOKEN)" \
	  --contract-did "$(CONTRACT_DID)" --signer-did "$(SIGNER_DID)" \
	  --field "$(FIELD)" --user "$(USER)"

$(PYTHON):
	python3 -m venv $(VENV_DIR)

$(VENV_STAMP): $(PYTHON) requirements.txt
	$(PIP) install -r requirements.txt
	touch $@

install: $(VENV_STAMP)

ensure-statuslist: $(VENV_STAMP)
	$(PYTHON) scripts/ensure_statuslist_for_dev.py

test: $(VENV_STAMP)
	$(PYTHON) -m unittest discover -s dcs_wallet -p "test_*.py"