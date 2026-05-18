build:
	go build -o bin/octo-version-sync ./main.go

run:
	go run ./main.go --store file --interval 1m

test:
	go test ./internal/ -v

clean:
	rm -rf bin/ output/
