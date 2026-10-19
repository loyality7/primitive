# Targets work in Git Bash / any POSIX shell with Go installed.
GOROOT := $(shell go env GOROOT)

.PHONY: build web test vet clean

build:            ## CLI -> bin/primitive
	go build -o bin/primitive ./cmd/primitive

web:              ## browser build -> web/primitive.wasm
	GOOS=js GOARCH=wasm go build -o web/primitive.wasm ./cmd/wasm
	cp "$(GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js

test:
	go test ./...

vet:
	go vet ./...
	GOOS=js GOARCH=wasm go vet ./cmd/wasm ./primitive

clean:
	rm -rf bin web/primitive.wasm web/wasm_exec.js
