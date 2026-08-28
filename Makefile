BINARY := peneira
BUNDLE := build/Peneira.app

VERSION ?= dev
LDFLAGS := -X main.version=$(VERSION)

STATICCHECK := honnef.co/go/tools/cmd/staticcheck@2025.1.1

.PHONY: all build run test vet fmt fmtcheck lint check app universal dist clean install

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/peneira

run: build
	./$(BINARY)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

fmtcheck:
	@out=$$(gofmt -l .); \
	if [ -n "$$out" ]; then echo "gofmt would change:"; echo "$$out"; exit 1; fi

lint:
	go run $(STATICCHECK) ./...

check: fmtcheck vet lint
	go build ./...
	go test ./...

app: build
	@packaging/bundle.sh $(BINARY) $(VERSION)

universal:
	mkdir -p build
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/peneira-arm64 ./cmd/peneira
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/peneira-amd64 ./cmd/peneira
	lipo -create -output build/$(BINARY) build/peneira-arm64 build/peneira-amd64
	rm -f build/peneira-arm64 build/peneira-amd64

dist: universal
	@packaging/bundle.sh build/$(BINARY) $(VERSION)
	rm -f build/Peneira-$(VERSION)-macos-universal.zip
	cd build && ditto -c -k --keepParent Peneira.app Peneira-$(VERSION)-macos-universal.zip
	cd build && tar czf peneira-$(VERSION)-macos-universal.tar.gz $(BINARY)
	@echo "built build/Peneira-$(VERSION)-macos-universal.zip"
	@echo "built build/peneira-$(VERSION)-macos-universal.tar.gz"

install: build
	install -d $(HOME)/.local/bin
	install -m 755 $(BINARY) $(HOME)/.local/bin/$(BINARY)

clean:
	rm -rf $(BINARY) build
