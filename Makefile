DROPBOX=~/Dropbox/Public/geodns

all:

linux: ccboot
	GOOS=linux \
	GOARCH=amd64 \
	go build -o $(DROPBOX)/cachestatus
	@echo "curl -sk https://dl.dropboxusercontent.com/u/25895/geodns/ccboot.sh | sh"

ccboot:
	cp ccboot.sh $(DROPBOX)
