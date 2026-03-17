build:
	go build -ldflags "-X main.version=$(shell git describe --tags --always)" -o claude-tab-fix .

fmt:
	gofmt -w .

install:
	go install .

release:
	goreleaser release --clean

release-snapshot:
	goreleaser release --snapshot --clean
