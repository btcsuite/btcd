# Developer Resources

* [Code Contribution Guidelines](code_contribution_guidelines.md)
* [Roadmap](ROADMAP.md)
* [M1 test plan](M1_TEST_PLAN.md)

* [JSON-RPC Reference](json_rpc_api.md)
  * [RPC Examples](json_rpc_api.md#ExampleCode)

* In-tree Bitcoin-related Go packages (module path still `github.com/btcsuite/btcd`):
  * [rpcclient](../rpcclient) — Websocket-enabled Bitcoin JSON-RPC client
  * [btcjson](../btcjson) — JSON-RPC command and return value APIs
  * [wire](../wire) — Bitcoin wire protocol
  * [peer](../peer) — Bitcoin network peers
  * [blockchain](../blockchain) — Block handling and chain selection
  * [blockchain/fullblocktests](../blockchain/fullblocktests) — Consensus validation test vectors
  * [txscript](../txscript) — Bitcoin transaction scripting language
  * [btcec](../btcec) — Elliptic curve crypto for Bitcoin scripts
  * [database](../database) — Database interface for the blockchain
  * [mempool](../mempool) — Policy-enforced pool of unmined transactions
  * [btcutil](../btcutil) — Bitcoin-specific convenience functions and types
  * [chainhash](../chainhash) — Generic hash type and helpers
  * [connmgr](../connmgr) — Generic Bitcoin network connection manager
  * [blockcompress](../blockcompress) — Cold-tier zstd codec (M1)
