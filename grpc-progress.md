
TODOs:

 - [x] implement listening
 - [ ] add GRPC dependenies (waiting for dep PR)
 - [ ] correct error handling
 - [ ] REST proxy? (cfr lnd)
 - [ ] in case it will replace the JSON-RPC interface: find a solution for
       getblocktemplate (maybe separate small HTTP server in the mining package 
	   behind `--getblocktemplate <interface>`)
 - [x] update rpcclient: there is a `WrappedBtcdClient` in the `btcrpc` package
	 that uses the respective Go structs like `chainhash.Hash`,
	 `btcutil.Tx|Block`, `wire.MsgTg`,...
 - [ ] update btcctl


Some questions:
 - is using `btcutil.Tx|Block.Serialize` safe instead of `BtcEncode`?


All current methods and the new methods that cover them:

 - [x] addnode: ConnectPeer
 - [x] createrawtransaction: deprecated
 - [x] debuglevel: SetDebugLevel
 - [x] decoderawtransaction: deprecated
 - [x] decodescript: deprecated
 - [x] generate: GenerateBlocks
 - [x] getaddednodeinfo: GetPeers
 - [x] getbestblock: GetBestBlockInfo
 - [x] getbestblockhash: GetBestBlockInfo
 - [x] getblock: GetBlock
 - [ ] **getblockchaininfo: could add to GetNetworkInfo**
 - [x] getblockcount: GetBestBlockInfo
 - [x] getblockhash: GetBlockInfo
 - [x] getblockheader: GetBlockInfo
 - [ ] **getblocktemplate**: see below
 - [x] getconnectioncount: GetSystemInfo
 - [x] getcurrentnet: GetNetworkInfo
 - [x] getdifficulty: GetNetworkInfo or GetMiningInfo
 - [x] getgenerate: GetGenerate
 - [x] gethashespersec: GetHashRate (or GetNetworkInfo or GetMiningInfo for network HR)
 - [ ] **getheaders**
 - [x] getinfo: GetSystemInfo and GetNetworkInfo
 - [x] getmempoolinfo: GetMempoolInfo
 - [x] getmininginfo: GetMiningInfo
 - [x] getnettotals: GetSystemInfo
 - [x] getnetworkhashps: GetNetworkInfo or GetMiningInfo
 - [x] getpeerinfo: GetPeers
 - [x] getrawmempool: GetMempool and GetRawMempool
 - [x] getrawtransaction: GetRawTransactions
 - [x] gettxout: just use GetTransaction
 - [x] help: deprecated
 - [x] node: ConnectPeer, DisconnectPeer, GetPeers
 - [x] ping: deprecated
 - [ ] **searchrawtransactions: how is this different than rescan?**
 - [x] sendrawtransaction: SubmitTransactions
 - [x] setgenerate: SetGenerate
 - [x] stop: StopDaemon
 - [x] submitblock: SubmitBlock
 - [x] uptime: GetSystemInfoo
 - [x] validateaddress: deprecated
 - [ ] **verifychain**: what's the use case of this?
 - [x] verifymessage: deprecated
 - [x] version: GetSystemInfo

There are WebSocket-only commands:
 - [x] loadtxfilter: deprecated
 - [x] notifyblocks: SubscsribeBlocks
 - [x] notifynewtransactions: SubscribeTransactions
 - [x] notifyreceived: potentially SubscribeTransactions
 - [x] notifyspent: potentially SubscribeTransactions
 - [x] rescan: RescanTransactions
 - [ ] **rescanblocks: why is this useful?**
 - [x] session: deprecated
 - [x] stopnotifyblocks: SubscribeBlocks
 - [x] stopnotifynewtransactions: SubscribeTransaction
 - [x] stopnotifyspent: potentially SubscribeTransaction
 - [x] stopnotifyreceived: potentially SubscribeTransaction

Additions to the API that did not exist before:
 - GetAddressTransactions
 - GetAddressUnspentOutputs

These are mentioned in the code as not implemented, but desired:
 - estimatefee
 - estimatepriority
 - getchaintips
 - getmempoolentry: have GetRawMempool to get all
 - getnetworkinfo: GetNetworkInfo
 - getwork
 - invalidateblock
 - preciousblock
 - reconsiderblock
