


hello.wasm: ./cmd/hello/main.go
	cd cmd/hello && GOOS=js GOARCH=wasm go build -o hello.wasm
	mv cmd/hello/hello.wasm .
	wasm-strip hello.wasm

run: hello.wasm
	go run .

run_nix: hello.wasm
	nix-shell -p gcc --run "go run ."

test_exec:
	# TODO
	GOOS=js GOARCH=wasm go test -exec $(go env GOROOT)/misc/wasm/go_js_wasm_exec
