GOOS=linux GOARCH=amd64 go build -o bin/s3pd-linux-amd64
GOOS=darwin GARCH=arch64 go build -o bin/s3pd-darwin-arch64
GOOS=darwin GARCH=amd64	go build -o bin/s3pd-darwin-amd64
