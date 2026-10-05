FUZZTIME ?= 60s

.PHONY: test uitest gametest fuzz vet fmt cover

## test: run every test that needs neither a GPU nor a game installation
test:
	go test ./...

## uitest: also run the GPU tests, which open a hidden window; needs a display
uitest:
	go test -tags uitest -count=1 ./...

## gametest: also run the tests against a real installation, for example
##   make gametest PDX_GAME_DIR="C:/Steam/steamapps/common/Victoria 3/game"
## PDX_DUMP_DIR additionally writes decoded textures and rendered views out.
gametest:
	PDX_GAME_DIR="$(PDX_GAME_DIR)" go test -tags uitest -count=1 -v ./...

## fuzz: fuzz the mesh and texture readers for FUZZTIME each
fuzz:
	go test ./mesh -run '^$$' -fuzz FuzzRead -fuzztime $(FUZZTIME)
	go test ./texture -run '^$$' -fuzz FuzzDecode -fuzztime $(FUZZTIME)

## cover: show test coverage by package
cover:
	go test -cover ./...

vet:
	go vet ./...

fmt:
	go fmt ./...
