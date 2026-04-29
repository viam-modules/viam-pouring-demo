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
			--epochs 5 --batch-size 4 $(VLA_ARGS)'
	rsync -avz $(VLA_HOST):$(VLA_REMOTE_DIR)/openvla-finetuned ./
