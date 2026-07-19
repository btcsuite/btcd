# Update

* Run the following commands to update Bitcoin-Praxis, all dependencies, and rebuild:

```bash
cd /path/to/bitcoin-praxis
git pull
go build -o praxisd .
go build -o praxctl ./cmd/praxctl
# or: make build / make install
```
