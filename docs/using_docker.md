# Using Docker

- [Using Docker](#using-docker)
  - [Introduction](#introduction)
  - [Docker volumes](#docker-volumes)
  - [Known error messages when starting the praxisd container](#known-error-messages-when-starting-the-praxisd-container)
  - [Examples](#examples)
    - [Preamble](#preamble)
    - [Full node without RPC port](#full-node-without-rpc-port)
    - [Full node with RPC port](#full-node-with-rpc-port)
    - [Full node with RPC port running on TESTNET](#full-node-with-rpc-port-running-on-testnet)

## Introduction

With Docker you can set up **praxisd** (Bitcoin-Praxis) to run a Bitcoin full node.
Build from this repository's Dockerfile (or compose `build:` against the GitHub
URL). Upstream btcd images on Docker Hub ([btcsuite/btcd](https://hub.docker.com/r/btcsuite/btcd))
are not Bitcoin-Praxis builds.

This documentation focuses on running containers with *docker-compose.yml*
files. For more information see the official [Docker documentation](https://docs.docker.com/).

## Docker volumes

**Disk space:** The examples use a Docker managed volume named *praxisd-data*.
This will use a lot of disk space (full chain, or cold-tier compacted chain with
`--witness-buffer`). See [Docker volumes](https://docs.docker.com/storage/volumes/).

The *praxisd-data* volume is reused across compose upgrades. It is not removed
when you delete the container. Delete it manually if you no longer need it:

```bash
docker volume ls
docker volume rm praxisd-data
```

## Known error messages when starting the praxisd container

When all options are passed on the command line, praxisd may still log that a
default config file is missing. These can be ignored:

```bash
Error creating a default config file: open /sample-praxisd.conf: no such file or directory
...
[WRN] PRXD: open /root/.praxisd/praxisd.conf: no such file or directory
```

## Examples

### Preamble

Defaults used below:

- `container_name: praxisd`
- `hostname: praxisd` — **set a fixed hostname before first start** so the RPC
  TLS certificate hostname stays stable across compose edits
- `restart: unless-stopped`
- Data dir in the container: `/root/.praxisd`

```bash
mkdir ~/praxisd-docker
cd ~/praxisd-docker
# create docker-compose.yml from an example below
docker-compose up
```

Useful commands:

```bash
docker-compose up -d
docker-compose down   # volume praxisd-data is kept
docker-compose stop
docker-compose start
docker logs praxisd
```

### Full node without RPC port

```yaml
version: "2"

services:
  praxisd:
    container_name: praxisd
    hostname: praxisd
    build: https://github.com/bitcoin-praxis/bitcoin-praxis.git#alt-node/compression
    restart: unless-stopped
    volumes:
      - praxisd-data:/root/.praxisd
    ports:
      - 8333:8333

volumes:
  praxisd-data:
```

### Full node with RPC port

```yaml
version: "2"

services:
  praxisd:
    container_name: praxisd
    hostname: praxisd
    build: https://github.com/bitcoin-praxis/bitcoin-praxis.git#alt-node/compression
    restart: unless-stopped
    volumes:
      - praxisd-data:/root/.praxisd
    ports:
      - 8333:8333
      - 8334:8334
    command: [
        "--rpcuser=[CHOOSE_A_USERNAME]",
        "--rpcpass=[CREATE_A_VERY_HARD_PASSWORD]"
    ]

volumes:
  praxisd-data:
```

### Full node with RPC port running on TESTNET

Testnet ports are 18333 (p2p) and 18334 (RPC):

```yaml
version: "2"

services:
  praxisd:
    container_name: praxisd
    hostname: praxisd
    build: https://github.com/bitcoin-praxis/bitcoin-praxis.git#alt-node/compression
    restart: unless-stopped
    volumes:
      - praxisd-data:/root/.praxisd
    ports:
      - 18333:18333
      - 18334:18334
    command: [
        "--testnet",
        "--rpcuser=[CHOOSE_A_USERNAME]",
        "--rpcpass=[CREATE_A_VERY_HARD_PASSWORD]"
    ]

volumes:
  praxisd-data:
```
