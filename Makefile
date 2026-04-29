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

VLA_VENV := cmd/vla/.venv
VLA_PY := $(VLA_VENV)/bin/python3
# OpenVLA's pinned stack (transformers 4.40, torch ~2.2) does not run on
# Python >=3.13. Override with VLA_PYTHON=python3.x if needed.
VLA_PYTHON ?= python3.11

$(VLA_VENV):
	$(VLA_PYTHON) -m venv $@

$(VLA_VENV)/.installed: cmd/vla/requirements.txt | $(VLA_VENV)
	$(VLA_VENV)/bin/pip install --upgrade pip
	$(VLA_VENV)/bin/pip install -r cmd/vla/requirements.txt
	touch $@

# Add --load-4bit (Linux/CUDA only) via: make vlagen VLA_ARGS=--load-4bit
VLA_ARGS ?=

vlagen: $(VLA_VENV)/.installed
	$(VLA_PY) cmd/vla/train_openvla.py --data-root openvla-export --output-dir openvla-finetuned --epochs 5 --batch-size 4 $(VLA_ARGS)
