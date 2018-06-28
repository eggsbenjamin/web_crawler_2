all: deps test bench

deps:
	go get -u github.com/golang/dep/cmd/dep
	dep ensure -v

test:
	go test ./...

bench:
	go test ./... -bench=.
