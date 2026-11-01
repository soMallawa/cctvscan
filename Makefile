.PHONY: build test vet clean test-console test-console-integration test-console-e2e

test-console:
	python3 -m unittest discover -s qa/console -p 'test_*.py'
	go test ./internal/controlplane/... ./internal/events/...
	npm --prefix web test
	npm --prefix web run build

test-console-integration:
	go test -tags=integration ./internal/controlplane/... ./cmd/cctvscan-server ./cmd/cctvscan-worker -count=1

test-console-e2e:
	go build -o bin/cctvscan ./cmd/cctvscan
	CCTVSCAN_TEST_SCANNER_BIN="$(CURDIR)/bin/cctvscan" go test -v -tags="integration scanner_e2e" ./internal/controlplane/runner -run '^TestRealScannerThroughDurableQueue$$' -count=1
	npm --prefix web run test:e2e
build:
	mkdir -p bin
	go build -trimpath -o bin/cctvscan ./cmd/cctvscan
test:
	go test -race ./...
vet:
	go vet ./...
clean:
	rm -f bin/cctvscan
