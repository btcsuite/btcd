# Update

* Run the following commands to update btcd, all dependencies, and install it:

```bash
cd $GOPATH/src/github.com/dashevo/dashd-go
git pull && GO111MODULE=on go install -v . ./cmd/...
```
