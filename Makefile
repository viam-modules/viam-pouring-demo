mac:
	rm -rf bin
	GOOS=darwin GOARCH=arm64 go build -o bin/darwin-arm64/viam-pouring-demo

linux:
	rm -rf bin
	GOOS=linux GOARCH=amd64 go build -o bin/linux-amd64/viam-pouring-demo

update-rdk:
	go get go.viam.com/rdk@latest
	go mod tidy