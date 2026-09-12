BINARY := check
BUNDLE := build/Check.app

VERSION ?= dev
LDFLAGS := -X main.version=$(VERSION)

STATICCHECK := honnef.co/go/tools/cmd/staticcheck@2026.2.1

.PHONY: all build run test vet fmt fmtcheck lint check app universal dist clean install

all: build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/check

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
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o build/check-arm64 ./cmd/check
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o build/check-amd64 ./cmd/check
	lipo -create -output build/$(BINARY) build/check-arm64 build/check-amd64
	rm -f build/check-arm64 build/check-amd64

dist: universal
	@packaging/bundle.sh build/$(BINARY) $(VERSION)
	rm -f build/Check-$(VERSION)-macos-universal.zip
	cd build && ditto -c -k --keepParent Check.app Check-$(VERSION)-macos-universal.zip
	cd build && tar czf check-$(VERSION)-macos-universal.tar.gz $(BINARY)
	@echo "built build/Check-$(VERSION)-macos-universal.zip"
	@echo "built build/check-$(VERSION)-macos-universal.tar.gz"

install: build
	install -d $(HOME)/.local/bin
	install -m 755 $(BINARY) $(HOME)/.local/bin/$(BINARY)

clean:
	rm -rf $(BINARY) build
