build:
	go build -o claude-tab-fix .

fmt:
	gofmt -w .

install:
	go install .

release:
	goreleaser release --clean

release-snapshot:
	goreleaser release --snapshot --clean
