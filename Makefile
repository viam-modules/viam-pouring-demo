GO_BUILD_ENV :=
GO_BUILD_FLAGS :=
MODULE_BINARY := bin/viam-pouring-demo

ifeq ($(VIAM_TARGET_OS), windows)
	GO_BUILD_ENV += GOOS=windows GOARCH=amd64
	GO_BUILD_FLAGS := -tags no_cgo	
	MODULE_BINARY = bin/viam-pouring-demo.exe
endif

$(MODULE_BINARY): bin Makefile go.mod cmd/module/*.go pour/*.go pour/vinoweb/dist/index.html
	$(GO_BUILD_ENV) go build $(GO_BUILD_FLAGS) -o $(MODULE_BINARY) cmd/module/main.go

bin:
	mkdir -p $@


lint:
	gofmt -s -w .

update:
	go get go.viam.com/rdk@latest
	go get github.com/erh/vmodutils@latest
	go mod tidy

test: pour/vinoweb/dist/index.html
	$(GO_BUILD_ENV) go test ./...

module.tar.gz: test meta.json $(MODULE_BINARY)
ifeq ($(VIAM_TARGET_OS), windows)
	jq '.entrypoint = "./bin/viam-pouring-demo.exe"' meta.json > temp.json && mv temp.json meta.json
else
	strip $(MODULE_BINARY)
endif
	tar czf $@ meta.json $(MODULE_BINARY) pour/vinoweb/dist
ifeq ($(VIAM_TARGET_OS), windows)
	git checkout meta.json
endif

module: test module.tar.gz

all: test module.tar.gz

setup:
	which apt > /dev/null 2>&1 && apt -y install nodejs || echo "no apt"

pour/vinoweb/dist/index.html: pour/vinoweb/*.json pour/vinoweb/*.html pour/vinoweb/src/*.ts pour/vinoweb/src/*.svelte pour/vinoweb/src/lib/*.svelte
	cd pour/vinoweb && npm install && npm run build

bin/tool: cmd/tools/*.go pour/*.go
	go build -o bin/tool cmd/tools/*.go

# vlagen runs training on a remote CUDA box reachable via ssh.
#   VLA_HOST       required, e.g. VLA_HOST=user@gpu-box
#   VLA_REMOTE_DIR remote workspace, default ~/viam-pouring-demo-vla
#   VLA_ARGS       extra args to pass to train_openvla.py (e.g. --load-4bit)
#
# Workflow: rsync script + dataset → run on remote in a venv → rsync model back.
VLA_REMOTE_DIR ?= ~/viam-pouring-demo-vla
VLA_ARGS ?=

vlagen:
	@if [ -z "$(VLA_HOST)" ]; then \
		echo "VLA_HOST is required (e.g. make vlagen VLA_HOST=user@gpu-box)"; \
		exit 1; \
	fi
	@if [ ! -d openvla-export ]; then \
		echo "no openvla-export/ here — capture some data first"; \
		exit 1; \
	fi
	ssh $(VLA_HOST) 'mkdir -p $(VLA_REMOTE_DIR)'
	rsync -avz \
		cmd/vla/train_openvla.py \
		cmd/vla/requirements.txt \
		openvla-export \
		$(VLA_HOST):$(VLA_REMOTE_DIR)/
	ssh $(VLA_HOST) 'set -e; \
		cd $(VLA_REMOTE_DIR); \
		[ -d .venv ] || python3 -m venv .venv; \
		.venv/bin/pip install --upgrade pip; \
		.venv/bin/pip install -r requirements.txt; \
		.venv/bin/python3 -u train_openvla.py \
			--data-root openvla-export \
			--output-dir openvla-finetuned \
			--epochs 5 $(VLA_ARGS)'
	rsync -avz $(VLA_HOST):$(VLA_REMOTE_DIR)/openvla-finetuned ./

# vlarun runs the trained model on $(VLA_HOST) and drives a robot at $(ROBOT_HOST).
#   VLA_HOST           required, GPU box where the model + venv live
#   ROBOT_HOST         required, robot FQDN (passed as --host to the script)
#   VLA_REMOTE_DIR     remote workspace, default ~/viam-pouring-demo-vla
#   VIAM_API_KEY,
#   VIAM_API_KEY_ID    required (forwarded to the remote run)
#   VLA_MODEL_PATH     default openvla-finetuned/epoch_5
#   VLA_INFER_ARGS     extra args, e.g. VLA_INFER_ARGS="--max-steps 50 --instruction 'pick up the cup'"
VLA_MODEL_PATH ?= openvla-finetuned/epoch_5
VLA_INFER_ARGS ?=

vlarun:
	@if [ -z "$(VLA_HOST)" ]; then echo "VLA_HOST is required"; exit 1; fi
	@if [ -z "$(ROBOT_HOST)" ]; then echo "ROBOT_HOST is required"; exit 1; fi
	@if [ -z "$(VIAM_API_KEY)" ] || [ -z "$(VIAM_API_KEY_ID)" ]; then \
		echo "VIAM_API_KEY and VIAM_API_KEY_ID are required"; exit 1; \
	fi
	rsync -avz cmd/vla/infer_openvla.py $(VLA_HOST):$(VLA_REMOTE_DIR)/
	ssh $(VLA_HOST) "set -e; \
		cd $(VLA_REMOTE_DIR); \
		VIAM_API_KEY='$(VIAM_API_KEY)' VIAM_API_KEY_ID='$(VIAM_API_KEY_ID)' \
		.venv/bin/python3 -u infer_openvla.py \
			--model-path $(VLA_MODEL_PATH) \
			--host $(ROBOT_HOST) \
			$(VLA_INFER_ARGS)"

# vlarun-local runs inference on this machine — uses local openvla-finetuned/,
# local venv, and torch's CPU/MPS backend. Workable on M-series Macs ≥ 32GB.
VLA_VENV := cmd/vla/.venv
VLA_PY := $(VLA_VENV)/bin/python3
VLA_PYTHON ?= python3.11

$(VLA_VENV):
	$(VLA_PYTHON) -m venv $@

$(VLA_VENV)/.installed: cmd/vla/requirements.txt | $(VLA_VENV)
	$(VLA_VENV)/bin/pip install --upgrade pip
	$(VLA_VENV)/bin/pip install -r cmd/vla/requirements.txt
	touch $@

vlarun-local: $(VLA_VENV)/.installed
	@if [ -z "$(ROBOT_HOST)" ]; then echo "ROBOT_HOST is required"; exit 1; fi
	@if [ -z "$(VIAM_API_KEY)" ] || [ -z "$(VIAM_API_KEY_ID)" ]; then \
		echo "VIAM_API_KEY and VIAM_API_KEY_ID are required"; exit 1; \
	fi
	@if [ ! -d "$(VLA_MODEL_PATH)" ]; then \
		echo "model not found at $(VLA_MODEL_PATH); run 'make vlagen ...' first"; exit 1; \
	fi
	VIAM_API_KEY='$(VIAM_API_KEY)' VIAM_API_KEY_ID='$(VIAM_API_KEY_ID)' \
	$(VLA_PY) -u cmd/vla/infer_openvla.py \
		--model-path $(VLA_MODEL_PATH) \
		--host $(ROBOT_HOST) \
		$(VLA_INFER_ARGS)
