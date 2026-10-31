.PHONY: build test vet clean
build:
	mkdir -p bin
	go build -trimpath -o bin/cctvscan ./cmd/cctvscan
test:
	go test -race ./...
vet:
	go vet ./...
clean:
	rm -f bin/cctvscan
