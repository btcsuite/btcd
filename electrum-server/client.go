package electrum

import (
	"bufio"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
)

const (
	ClientVersion   = "0.0.1"
	ProtocolVersion = "1.4"
)

var (
	ErrNotImplemented = errors.New("not implemented")
	ErrNodeConnected  = errors.New("node already connected")
	ClientName        = "go-electrum"
)

type Transport interface {
	SendMessage([]byte) error
	Responses() <-chan []byte
	Errors() <-chan error
}

type respMetadata struct {
	Id     int    `json:"id"`
	Method string `json:"method"`
	Error  string `json:"error"`
}

type request struct {
	Id     int      `json:"id"`
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type basicResp struct {
	Result string `json:"result"`
}

type Node struct {
	Address string

	transport    Transport
	handlers     map[int]chan []byte
	handlersLock sync.RWMutex

	pushHandlers     map[string][]chan []byte
	pushHandlersLock sync.RWMutex

	nextId int
}

type TCPTransport struct {
	conn      net.Conn
	responses chan []byte
	errors    chan error
}

func NewTCPTransport(addr string) (*TCPTransport, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	t := &TCPTransport{
		conn:      conn,
		responses: make(chan []byte),
		errors:    make(chan error),
	}
	go t.listen()
	return t, nil
}

func NewSSLTransport(addr string, config *tls.Config) (*TCPTransport, error) {
	conn, err := tls.Dial("tcp", addr, config)
	if err != nil {
		return nil, err
	}
	t := &TCPTransport{
		conn:      conn,
		responses: make(chan []byte),
		errors:    make(chan error),
	}
	go t.listen()
	return t, nil
}

func (t *TCPTransport) SendMessage(body []byte) error {
	log.Debugf("%s <- %s", t.conn.RemoteAddr(), body)
	_, err := t.conn.Write(body)
	return err
}

func (t *TCPTransport) listen() {
	defer t.conn.Close()
	reader := bufio.NewReader(t.conn)
	for {
		fmt.Println("wait")
		line, err := reader.ReadBytes(delim)
		fmt.Println("read", line)
		if err != nil {
			t.errors <- fmt.Errorf("TCPTransport Readbytes error %s, read %d", err, len(line))
			log.Debugf("TCPTransport Readbytes error %s", err)
			break
		}
		log.Debugf("%s -> %s", t.conn.RemoteAddr(), line)
		t.responses <- line
	}
}

func (t *TCPTransport) Responses() <-chan []byte {
	return t.responses
}
func (t *TCPTransport) Errors() <-chan error {
	return t.errors
}

// NewNode creates a new node.
func NewNode() *Node {
	n := &Node{
		handlers:     make(map[int]chan []byte),
		pushHandlers: make(map[string][]chan []byte),
	}
	return n
}

// ConnectTCP creates a new TCP connection to the specified address.
func (n *Node) ConnectTCP(addr string) error {
	if n.transport != nil {
		return ErrNodeConnected
	}
	n.Address = addr
	transport, err := NewTCPTransport(addr)
	if err != nil {
		return err
	}
	n.transport = transport
	go n.listen()
	return nil
}

// ConnectSSL creates a new SSL connection to the specified address.
func (n *Node) ConnectSSL(addr string, config *tls.Config) error {
	if n.transport != nil {
		return ErrNodeConnected
	}
	n.Address = addr
	transport, err := NewSSLTransport(addr, config)
	if err != nil {
		return err
	}
	n.transport = transport
	go n.listen()
	return nil
}

// err handles errors produced by the foreign node.
func (n *Node) err(err error) {
	// TODO (d4l3k) Better error handling.
	log.Critical(err)
}

// listen processes messages from the server.
func (n *Node) listen() {
	for {
		select {
		case err := <-n.transport.Errors():
			n.err(fmt.Errorf("Node.listen error: %v", err))
			return
		case bytes := <-n.transport.Responses():
			msg := &respMetadata{}
			if err := json.Unmarshal(bytes, msg); err != nil {
				n.err(fmt.Errorf("Node.listen json unmarshall error: %v", err))
				return
			}
			if len(msg.Error) > 0 {
				n.err(fmt.Errorf("error from server: %#v", msg.Error))
				return
			}
			if len(msg.Method) > 0 {
				n.pushHandlersLock.RLock()
				handlers := n.pushHandlers[msg.Method]
				n.pushHandlersLock.RUnlock()

				for _, handler := range handlers {
					select {
					case handler <- bytes:
					default:
					}
				}
			}

			n.handlersLock.RLock()
			c, ok := n.handlers[msg.Id]
			n.handlersLock.RUnlock()

			if ok {
				c <- bytes
			}
		}
	}
}

// listenPush returns a channel of messages matching the method.
func (n *Node) listenPush(method string) <-chan []byte {
	c := make(chan []byte, 1)
	n.pushHandlersLock.Lock()
	defer n.pushHandlersLock.Unlock()
	n.pushHandlers[method] = append(n.pushHandlers[method], c)
	return c
}

// request makes a request to the server and unmarshals the response into v.
func (n *Node) request(method string, params []string, v interface{}) error {
	msg := request{
		Id:     n.nextId,
		Method: method,
		Params: params,
	}
	n.nextId++
	bytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	bytes = append(bytes, delim)
	if err := n.transport.SendMessage(bytes); err != nil {
		return err
	}

	c := make(chan []byte, 1)

	n.handlersLock.Lock()
	n.handlers[msg.Id] = c
	n.handlersLock.Unlock()

	resp := <-c

	n.handlersLock.Lock()
	defer n.handlersLock.Unlock()
	delete(n.handlers, msg.Id)

	if err := json.Unmarshal(resp, v); err != nil {
		return nil
	}
	return nil
}

//type HostPort struct {
//	TCPPort int `json:"tcp_port"`
//	SslPort int `json:"ssl_port"`
//}
//
//type ServerFeaturesResponse struct {
//	GenesisHash   string              `json:"genesis_hash"`
//	Hosts         map[string]HostPort `json:"hosts"`
//	ProtocolMax   string              `json:"protocol_max"`
//	ProtocolMin   string              `json:"protocol_min"`
//	Pruning       bool                `json:"pruning,omitempty"`
//	ServerVersion string              `json:"server_version"`
//	HashFunction  string              `json:"hash_function"`
//}

func (n *Node) ServerFeatures() (*ServerFeaturesResponse, error) {
	resp := &struct {
		Result *ServerFeaturesResponse `json:"result"`
	}{}
	err := n.request("server.features", []string{}, resp)
	return resp.Result, err
}

func (n *Node) Ping() error {
	resp := &basicResp{}
	err := n.request("server.ping", []string{}, resp)
	return err
}

type BlockchainGetBlockHeaderResponse struct {
	Branch []string `json:"branch"`
	Header string   `json:"header"`
	Root   string   `json:"root"`
}

func (n *Node) BlockchainGetBlockHeader(height int) (string, error) {
	resp := &struct {
		Result string `json:"result"`
	}{}
	err := n.request("blockchain.block.header", []string{strconv.Itoa(height)}, &resp)

	return resp.Result, err
}

//type HeadersResponse struct {
//	Count int    `json:"count"`
//	Hex   string `json:"hex"`
//	Max   int    `json:"max"`
//}

func (n *Node) BlockchainGetBlockHeaders(start, count int) (*HeadersResponse, error) {
	resp := &struct {
		Result *HeadersResponse `json:"result"`
	}{}
	err := n.request("blockchain.block.headers",
		[]string{strconv.Itoa(start), strconv.Itoa(count)}, &resp)

	return resp.Result, err
}

// BlockchainNumBlocksSubscribe returns the current number of blocks.
// http://docs.electrum.org/en/latest/protocol.html#blockchain-numblocks-subscribe
func (n *Node) BlockchainNumBlocksSubscribe() (int, error) {
	resp := &struct {
		Result int `json:"result"`
	}{}
	err := n.request("blockchain.numblocks.subscribe", nil, resp)
	return resp.Result, err
}

type BlockchainHeader struct {
	Nonce         uint64 `json:"nonce"`
	PrevBlockHash string `json:"prev_block_hash"`
	Timestamp     uint64 `json:"timestamp"`
	MerkleRoot    string `json:"merkle_root"`
	BlockHeight   uint64 `json:"block_height"`
	UtxoRoot      string `json:"utxo_root"`
	Version       int    `json:"version"`
	Bits          uint64 `json:"bits"`
}

// BlockchainHeadersSubscribe request client notifications about new blocks in
// form of parsed blockheaders and returns the current block header.
// http://docs.electrum.org/en/latest/protocol.html#blockchain-headers-subscribe
func (n *Node) BlockchainHeadersSubscribe() (<-chan *BlockchainHeader, error) {
	resp := &struct {
		Result *BlockchainHeader `json:"result"`
	}{}
	if err := n.request("blockchain.headers.subscribe", []string{}, resp); err != nil {
		return nil, err
	}
	headerChan := make(chan *BlockchainHeader, 1)
	headerChan <- resp.Result
	go func() {
		for msg := range n.listenPush("blockchain.headers.subscribe") {
			resp := &struct {
				Params []*BlockchainHeader `json:"params"`
			}{}
			if err := json.Unmarshal(msg, resp); err != nil {
				log.Errorf("BlockchainHeadersSubsricbe error: %s", err)
				return
			}
			for _, param := range resp.Params {
				headerChan <- param
			}
		}
	}()
	return headerChan, nil
}

// BlockchainScriptHashSubscribe subscribes to transactions on an address and
// returns the hash of the transaction history.
// http://docs.electrum.org/en/latest/protocol.html#blockchain-address-subscribe
func (n *Node) BlockchainScriptHashSubscribe(address string) (<-chan string, error) {
	addr, err := btcutil.DecodeAddress(address, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}
	pkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(pkScript)
	// Reverse the hash.
	sort.SliceStable(hash[:], func(i, j int) bool {
		return i > j
	})

	hashStr := hex.EncodeToString(hash[:])
	fmt.Printf("scriptHash: %v\n", hashStr)
	resp := &basicResp{}
	err = n.request("blockchain.scripthash.subscribe", []string{hashStr}, resp)
	if err != nil {
		return nil, err
	}
	addressChan := make(chan string, 1)
	if len(resp.Result) > 0 {
		addressChan <- resp.Result
	}
	go func() {
		for msg := range n.listenPush("blockchain.scripthash.subscribe") {
			resp := &struct {
				Params []string `json:"params"`
			}{}
			if err := json.Unmarshal(msg, resp); err != nil {
				log.Errorf("BlockchainScriptHashSubscribe error: %s", err)
				return
			}
			if len(resp.Params) != 2 {
				log.Debugf("address subscription params len != 2 %+v", resp.Params)
				continue
			}
			if resp.Params[0] == address {
				addressChan <- resp.Params[1]
			}
		}
	}()
	return addressChan, err
}

type Transaction struct {
	Hash   string `json:"tx_hash"`
	Height int    `json:"height"`
	Value  int    `json:"value"`
	Pos    int    `json:"tx_pos"`
}

// BlockchainAddressGetHistory returns the history of an address.
// http://docs.electrum.org/en/latest/protocol.html#blockchain-address-get-history
func (n *Node) BlockchainAddressGetHistory(address string) ([]*Transaction, error) {
	resp := &struct {
		Result []*Transaction `json:"result"`
	}{}
	err := n.request("blockchain.address.get_history", []string{address}, resp)
	return resp.Result, err
}

// TODO(d4l3k) implement
// http://docs.electrum.org/en/latest/protocol.html#blockchain-address-get-mempool
func (n *Node) BlockchainAddressGetMempool() error { return ErrNotImplemented }

//type Balance struct {
//	Confirmed   btcutil.Amount `json:"confirmed"`
//	Unconfirmed btcutil.Amount `json:"unconfirmed"`
//}

// BlockchainScriptHashGetBalance returns the balance of an script hash.
func (n *Node) BlockchainScriptHashGetBalance(scriptHash string) (*Balance, error) {
	resp := &struct {
		Result *Balance `json:"result"`
	}{}
	err := n.request("blockchain.scripthash.get_balance", []string{scriptHash}, resp)
	return resp.Result, err
}

// BlockchainAddressGetBalance is a wrapper around BlockchainScriptHashGetBalance
// and it returns the balance of an address.
func (n *Node) BlockchainAddressGetBalance(address string) (*Balance, error) {
	addr, err := btcutil.DecodeAddress(address, &chaincfg.MainNetParams)
	if err != nil {
		return nil, err
	}
	pkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(pkScript)
	// Reverse the hash.
	sort.SliceStable(hash[:], func(i, j int) bool {
		return i > j
	})
	return n.BlockchainScriptHashGetBalance(hex.EncodeToString(hash[:]))
}

type History struct {
	Height int    `json:"height"`
	TxHash string `json:"tx_hash"`
}

// BlockchainScriptHashGetHistory returns the history of an script hash.
func (n *Node) BlockchainScriptHashGetHistory(scriptHash string) ([]*History, error) {
	resp := &struct {
		Result []*History `json:"result"`
	}{}
	err := n.request("blockchain.scripthash.get_history", []string{scriptHash}, resp)
	return resp.Result, err
}

func (n *Node) MempoolGetFeeHistogram() ([][]int, error) {
	resp := &struct {
		Result [][]int `json:"result"`
	}{}
	err := n.request("mempool.get_fee_histogram", []string{}, resp)
	return resp.Result, err
}

//// BlockchainScriptHashSubscribe
//func (n *Node) BlockchainScriptHashSubscribe(scriptHash string) (string, error) {
//	resp := &basicResp{}
//	err := n.request("blockchain.scripthash.subscribe", []string{scriptHash}, resp)
//	return resp.Result, err
//}

// TODO(d4l3k) implement
// http://docs.electrum.org/en/latest/protocol.html#blockchain-address-get-proof
func (n *Node) BlockchainAddressGetProof() error { return ErrNotImplemented }

// BlockchainAddressListUnspent lists the unspent transactions for the given address.
// http://docs.electrum.org/en/latest/protocol.html#blockchain-address-listunspent
func (n *Node) BlockchainAddressListUnspent(address string) ([]*Transaction, error) {
	resp := &struct {
		Result []*Transaction `json:"result"`
	}{}
	err := n.request("blockchain.scripthash.listunspent", []string{address}, resp)
	return resp.Result, err
}

// TODO(d4l3k) implement
// http://docs.electrum.org/en/latest/protocol.html#blockchain-utxo-get-address
func (n *Node) BlockchainUtxoGetAddress() error { return ErrNotImplemented }

// TODO(d4l3k) implement
// http://docs.electrum.org/en/latest/protocol.html#blockchain-block-get-header
func (n *Node) BlockchainBlockGetHeader() error { return ErrNotImplemented }

// TODO(d4l3k) implement
// http://docs.electrum.org/en/latest/protocol.html#blockchain-block-get-chunk
func (n *Node) BlockchainBlockGetChunk() error { return ErrNotImplemented }

// BlockchainTransactionBroadcast sends a raw transaction.
// TODO(d4l3k) implement
// http://docs.electrum.org/en/latest/protocol.html#blockchain-transaction-broadcast
func (n *Node) BlockchainTransactionBroadcast(tx []byte) (interface{}, error) {
	resp := &struct {
		Result interface{} `json:"result"`
	}{}
	err := n.request("blockchain.transaction.broadcast", []string{string(tx)}, resp)
	return resp.Result, err
}

type GetMerkle struct {
	Merkle      []string `json:"merkle"`
	BlockHeight int      `json:"block_height"`
	Pos         int      `json:"pos"`
}

// TODO(d4l3k) implement
// http://docs.electrum.org/en/latest/protocol.html#blockchain-transaction-get-merkle
func (n *Node) BlockchainTransactionGetMerkle(txHash chainhash.Hash, height int32) (*GetMerkle, error) {
	resp := &struct {
		Result *GetMerkle `json:"result"`
	}{}
	err := n.request("blockchain.transaction.get_merkle", []string{txHash.String(), strconv.Itoa(int(height))}, resp)
	return resp.Result, err
}

// BlockchainTransactionGet returns the raw transaction (hex-encoded) for the given txid. If transaction doesn't exist, an error is returned.
// http://docs.electrum.org/en/latest/protocol.html#blockchain-transaction-get
func (n *Node) BlockchainTransactionGet(txid string) (string, error) {
	resp := &basicResp{}
	err := n.request("blockchain.transaction.get", []string{txid}, resp)
	return resp.Result, err
}

// http://docs.electrum.org/en/latest/protocol.html#blockchain-estimatefee
// BlockchainEstimateFee estimates the transaction fee per kilobyte that needs to be paid for a transaction to be included within a certain number of blocks.
func (n *Node) BlockchainEstimateFee(block int) (float64, error) {
	resp := &struct {
		Result float64 `json:"result"`
	}{}
	err := n.request("blockchain.estimatefee", []string{strconv.Itoa(block)}, resp)
	return resp.Result, err
}

// ServerVersion returns the server's version.
// http://docs.electrum.org/en/latest/protocol.html#server-version
func (n *Node) ServerVersion() (string, error) {
	resp := &basicResp{}
	err := n.request("server.version", []string{ClientVersion, ProtocolVersion}, resp)
	return resp.Result, err
}

// ServerBanner returns the server's banner.
// http://docs.electrum.org/en/latest/protocol.html#server-banner
func (n *Node) ServerBanner() (string, error) {
	resp := &basicResp{}
	err := n.request("server.banner", nil, resp)
	return resp.Result, err
}

// ServerDonationAddress returns the donation address of the server.
// http://docs.electrum.org/en/latest/protocol.html#server-donation-address
func (n *Node) ServerDonationAddress() (string, error) {
	resp := &basicResp{}
	err := n.request("server.donation_address", []string{}, resp)
	return resp.Result, err
}

// ServerPeersSubscribe requests peers from a server.
// http://docs.electrum.org/en/latest/protocol.html#server-peers-subscribe
func (n *Node) ServerPeersSubscribe() ([][]interface{}, error) {
	resp := &struct {
		Peers [][]interface{} `json:"result"`
	}{}
	err := n.request("server.peers.subscribe", nil, resp)
	return resp.Peers, err
}
