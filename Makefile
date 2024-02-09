.PHONY: build preprocess clean install install-release



preprocess: 
	echo "using makefile"
	@echo "Preprocessing..."
	go build ./cmd/ebbuilder
	./ebbuilder 
	@-rm -f ebbuilder
	@-del  .\ebbuilder.exe
	@echo "Done."



build:preprocess
	@echo "Building..."
	go build ./cmd/ebcli
	go build ./cmd/eb
	@echo "Done."

clean:
	@echo "Cleaning..."
	@-rm -f eb
	@-rm -f ebbuilder
	@-rm -f ebcli
	@-del  .\eb.exe
	@-del  .\ebbuilder.exe
	@-del  .\ebcli.exe
	@echo "Done."

install:preprocess
	@echo "Installing..."
	go install ./cmd/eb
	go install ./cmd/ebcli
	go install ./cmd/ebbuilder
	@echo "Done."

install-release:preprocess
	@echo "Installing..."
	go install -ldflags "-s -w" -tags=release ./cmd/eb
	go install -ldflags "-s -w" -tags=release ./cmd/ebcli
	go install -ldflags "-s -w" -tags=release ./cmd/ebbuilder
	@echo "Done."