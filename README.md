# tinygo-blog
Blog using TinyGo + WASM
```
tinygo build -gc=leaking -no-debug -o main.wasm -target wasm main.go
gzip -9 main.wasm
python3 -m http.server 8080
```
