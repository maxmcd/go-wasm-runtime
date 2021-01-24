


hello.wasm: ./cmd/hello/main.go
	cd cmd/hello && GOOS=js GOARCH=wasm go build -o hello.wasm
	mv cmd/hello/hello.wasm .
	wasm-strip hello.wasm

run: hello.wasm
	nix-shell -p gcc --run "go run ."
