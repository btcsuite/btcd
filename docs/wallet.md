# Wallet

Upstream btcd intentionally shipped without an integrated wallet. Bitcoin-Praxis
does **not** keep that as the end state.

**M3 (planned):** embed [btcwallet](https://github.com/btcsuite/btcwallet)
in-process under `--enable-wallet`, with wallet state under
`~/.praxisd/wallet/`, and ship **Bitcoin-Praxis Wallet** — a native GioUI GUI
in the same `praxisd` binary (no loopback RPC, no web view).

Until M3 lands, use btcwallet (or another wallet) against the node's RPC as with
upstream btcd. See [ROADMAP.md](ROADMAP.md) for scope and acceptance criteria,
including witness-excision compatibility for cold-tier rescans.
