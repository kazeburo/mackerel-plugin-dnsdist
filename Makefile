VERSION=0.0.4
LDFLAGS=-ldflags "-w -s -X main.version=${VERSION}"
all: mackerel-plugin-dnsdist

.PHONY: mackerel-plugin-dnsdist

mackerel-plugin-dnsdist: cmd/mackerel-plugin-dnsdist/main.go
	go build $(LDFLAGS) -o mackerel-plugin-dnsdist cmd/mackerel-plugin-dnsdist/main.go

linux: cmd/mackerel-plugin-dnsdist/main.go
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o mackerel-plugin-dnsdist cmd/mackerel-plugin-dnsdist/main.go

fmt:
	go fmt ./...

check:
	go test ./...

clean:
	rm -rf mackerel-plugin-dnsdist

tag:
	git tag v${VERSION}
	git push origin v${VERSION}
	git push origin main