# Gorun

Run Go programs as scripts with build cache. It sets GOPROXY environment to local Go cache directory before build.

## Usage

```
gorun file-1.go,file-2.go,... [arguments]
```

## Build (requires Go 1.21+)

```
git clone https://github.com/vs022/gorun.git
cd gorun
go mod init gorun
go build
```
