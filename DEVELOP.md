# Develop doc
Go version >= 1.11 is required, after this version you don’t need to set the GOPATH variable.


## Install the Go environment
Install Go on Mac

```bash
brew install go
```

## compile method
Compilation reference: https://github.com/golang/go/wiki/GoArm

```bash
# download code
git clone https://github.com/openatx/atx-agent
cd atx-agent

# The proxy can be set through the following command, which is convenient for domestic users. Foreign users ignore
export GOPROXY=https://goproxy.io

# Use go.mod to manage dependent libraries
export GO111MODULE=on

# Package the files in the assets directory into go code
go get -v github.com/shurcooL/vfsgen # It doesn't matter if you don't execute this
go generate

# build for android binary
GOOS=linux GOARCH=arm go build -tags vfs
```

## seven cows
Thanks to ken for the Qiniu mirror service. By default, the qiniu server will go to github to pull the mirror, but because the mirror service has become more and more unstable recently (2020-03-20), it is currently changed to push directly to Qiniu CDN on the travis server