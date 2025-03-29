GO_BUILD_ENV :=
GO_BUILD_FLAGS :=
MODULE_BINARY := bin/viam-pouring-demo

ifeq ($(VIAM_TARGET_OS), windows)
	GO_BUILD_ENV += GOOS=windows GOARCH=amd64
	GO_BUILD_FLAGS := -tags no_cgo	
	MODULE_BINARY = bin/viam-pouring-demo.exe
endif

$(MODULE_BINARY): bin Makefile go.mod cmd/module/*.go pour/*.go pour/web/dist/index.html
	$(GO_BUILD_ENV) go build $(GO_BUILD_FLAGS) -o $(MODULE_BINARY) cmd/module/main.go

bin:
	mkdir -p $@


lint:
	gofmt -s -w .

update:
	go get go.viam.com/rdk@latest
	go mod tidy

test: pour/web/dist/index.html
	$(GO_BUILD_ENV) go test ./...

module.tar.gz: test meta.json $(MODULE_BINARY)
ifeq ($(VIAM_TARGET_OS), windows)
	jq '.entrypoint = "./bin/viam-pouring-demo.exe"' meta.json > temp.json && mv temp.json meta.json
else
	strip $(MODULE_BINARY)
endif
	tar czf $@ meta.json $(MODULE_BINARY)
ifeq ($(VIAM_TARGET_OS), windows)
	git checkout meta.json
endif

module: test module.tar.gz

all: test module.tar.gz

setup:
	which apt > /dev/null 2>&1 && apt -y install nodejs || echo "no apt"

pour/web/dist/index.html: pour/web/*.json pour/web/*.html pour/web/src/*.ts pour/web/src/*.svelte
	cd pour/web && npm install && npm run build
