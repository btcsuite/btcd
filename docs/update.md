# Update

* Run the following commands to update btcd, all dependencies, and install it:

```bash
cd $GOPATH/src/github.com/btcsuite/btcd
git pull && go install -v . ./cmd/...
```
