package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcrpc"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/mining"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"google.golang.org/grpc"
)

type Client struct {
	cc *grpc.ClientConn

	btcd btcrpc.BtcdClient
}

func NewClient(grpcConn *grpc.ClientConn) *Client {
	return &Client{
		cc:   grpcConn,
		btcd: btcrpc.NewBtcdClient(grpcConn),
	}
}

func ensureCtx(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.TODO()
}

// SYSTEM MANAGEMENT

func (c *Client) GetSystemInfo(ctx context.Context, opts ...grpc.CallOption) (*btcrpc.GetSystemInfoResponse, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetSystemInfoRequest{}
	return c.btcd.GetSystemInfo(ctx, req, opts...)
}

func (c *Client) SetDebugLevel(ctx context.Context, level, subsystem string, opts ...grpc.CallOption) error {
	ctx = ensureCtx(ctx)

	lvl, found := btcrpc.SetDebugLevelRequest_DebugLevel_value[level]
	if !found {
		return fmt.Errorf("Debug level '%v' does not exist", level)
	}
	ss, found := btcrpc.SetDebugLevelRequest_Subsystem_value[subsystem]
	if !found {
		return fmt.Errorf("Subsystem '%v' does not exist", subsystem)
	}

	req := &btcrpc.SetDebugLevelRequest{
		Level:     btcrpc.SetDebugLevelRequest_DebugLevel(lvl),
		Subsystem: btcrpc.SetDebugLevelRequest_Subsystem(ss),
	}
	_, err := c.btcd.SetDebugLevel(ctx, req, opts...)
	return err
}

func (c *Client) StopDaemon(ctx context.Context, opts ...grpc.CallOption) error {
	ctx = ensureCtx(ctx)

	req := &btcrpc.StopDaemonRequest{}
	_, err := c.btcd.StopDaemon(ctx, req, opts...)
	return err
}

// NETWORK

func (c *Client) GetNetworkInfo(ctx context.Context, opts ...grpc.CallOption) (*btcrpc.GetNetworkInfoResponse, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetNetworkInfoRequest{}
	return c.btcd.GetNetworkInfo(ctx, req, opts...)
}

func (c *Client) CalculateNetworkHashRate(ctx context.Context, endHeight, nbBlocks int32, opts ...grpc.CallOption) (uint64, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.CalculateNetworkHashRateRequest{
		EndHeight: endHeight,
		NbBlocks:  nbBlocks,
	}
	ret, err := c.btcd.CalculateNetworkHashRate(ctx, req, opts...)
	return ret.GetHashrate(), err
}

// MEMPOOL

func (c *Client) GetMempoolInfo(ctx context.Context, opts ...grpc.CallOption) (*btcrpc.GetMempoolInfoResponse, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetMempoolInfoRequest{}
	return c.btcd.GetMempoolInfo(ctx, req, opts...)
}

func (c *Client) GetMempoolHashes(ctx context.Context, opts ...grpc.CallOption) ([]*chainhash.Hash, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetMempoolRequest{FullTransactions: false}
	ret, err := c.btcd.GetMempool(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	hashes := make([]*chainhash.Hash, len(ret.GetTransactionHashes()))
	for i, h := range ret.GetTransactionHashes() {
		hashes[i], err = chainhash.NewHash(h)
		if err != nil {
			return nil, err
		}
	}
	return hashes, nil
}

func (c *Client) GetMempoolTxs(ctx context.Context, opts ...grpc.CallOption) ([]*btcrpc.MempoolTransaction, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetMempoolRequest{FullTransactions: true}
	ret, err := c.btcd.GetMempool(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	return ret.GetTransactions(), nil
}

func (c *Client) GetRawMempool(ctx context.Context, opts ...grpc.CallOption) ([]*btcutil.Tx, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetRawMempoolRequest{}
	ret, err := c.btcd.GetRawMempool(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	txs := make([]*btcutil.Tx, len(ret.GetTransactions()))
	for i, t := range ret.GetTransactions() {
		txs[i], err = btcutil.NewTxFromBytes(t)
		if err != nil {
			return nil, err
		}
	}
	return txs, nil
}

// BLOCKS

func (c *Client) GetBestBlockInfo(ctx context.Context, opts ...grpc.CallOption) (*btcrpc.BlockInfo, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetBestBlockInfoRequest{}
	ret, err := c.btcd.GetBestBlockInfo(ctx, req, opts...)
	return ret.GetInfo(), err
}

func (c *Client) GetBlockInfoByHash(ctx context.Context, hash *chainhash.Hash, opts ...grpc.CallOption) (*btcrpc.BlockInfo, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetBlockInfoRequest{Locator: btcrpc.NewBlockLocatorHash(hash)}
	ret, err := c.btcd.GetBlockInfo(ctx, req, opts...)
	return ret.GetInfo(), err
}

func (c *Client) GetBlockInfoByHeight(ctx context.Context, height int32, opts ...grpc.CallOption) (*btcrpc.BlockInfo, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetBlockInfoRequest{Locator: btcrpc.NewBlockLocatorHeight(height)}
	ret, err := c.btcd.GetBlockInfo(ctx, req, opts...)
	return ret.GetInfo(), err
}

func (c *Client) GetBlockByHash(ctx context.Context, hash *chainhash.Hash, opts ...grpc.CallOption) (*btcrpc.Block, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetBlockRequest{Locator: btcrpc.NewBlockLocatorHash(hash)}
	ret, err := c.btcd.GetBlock(ctx, req, opts...)
	return ret.GetBlock(), err
}

func (c *Client) GetBlockByHeight(ctx context.Context, height int32, opts ...grpc.CallOption) (*btcrpc.Block, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetBlockRequest{Locator: btcrpc.NewBlockLocatorHeight(height)}
	ret, err := c.btcd.GetBlock(ctx, req, opts...)
	return ret.GetBlock(), err
}

func (c *Client) GetRawBlockByHash(ctx context.Context, hash *chainhash.Hash, opts ...grpc.CallOption) (*btcutil.Block, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetRawBlockRequest{Locator: btcrpc.NewBlockLocatorHash(hash)}
	ret, err := c.btcd.GetRawBlock(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	return btcutil.NewBlockFromBytes(ret.GetBlock())
}

func (c *Client) GetRawBlockByHeight(ctx context.Context, height int32, opts ...grpc.CallOption) (*btcutil.Block, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetRawBlockRequest{Locator: btcrpc.NewBlockLocatorHeight(height)}
	ret, err := c.btcd.GetRawBlock(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	return btcutil.NewBlockFromBytes(ret.GetBlock())
}

func (c *Client) SubmitBlock(ctx context.Context, block *wire.MsgBlock, opts ...grpc.CallOption) (*chainhash.Hash, error) {
	ctx = ensureCtx(ctx)

	var buf bytes.Buffer
	if err := block.Serialize(&buf); err != nil {
		return nil, err
	}

	req := &btcrpc.SubmitBlockRequest{Block: buf.Bytes()}
	ret, err := c.btcd.SubmitBlock(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	return chainhash.NewHash(ret.GetHash())
}

// TRANSACTIONS

func (c *Client) GetTransaction(ctx context.Context, txHash *chainhash.Hash, opts ...grpc.CallOption) (*btcrpc.Transaction, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetTransactionRequest{Hash: txHash[:]}
	ret, err := c.btcd.GetTransaction(ctx, req, opts...)
	return ret.GetTransaction(), err
}

func (c *Client) GetRawTransaction(ctx context.Context, txHash *chainhash.Hash, opts ...grpc.CallOption) (*btcutil.Tx, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetRawTransactionRequest{Hash: txHash[:]}
	ret, err := c.btcd.GetRawTransaction(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	return btcutil.NewTxFromBytes(ret.GetTransaction())
}

type TransactionOrErr struct {
	Tx     *btcrpc.Transaction
	Height int32
	Err    error
}

func (c *Client) ScanTransactionsByHeights(ctx context.Context, startHeight, stopHeight int32, filter *btcrpc.TransactionFilter, opts ...grpc.CallOption) (<-chan TransactionOrErr, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.ScanTransactionsRequest{
		StartBlock: btcrpc.NewBlockLocatorHeight(startHeight),
		StopBlock:  btcrpc.NewBlockLocatorHeight(stopHeight),
		Filter:     filter,
	}
	str, err := c.btcd.ScanTransactions(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	ch := make(chan TransactionOrErr)
	go func() {
		for {
			update, err := str.Recv()
			if err == io.EOF {
				close(ch)
				return
			} else if err != nil {
				ch <- TransactionOrErr{Err: err}
				continue
			}

			ch <- TransactionOrErr{
				Tx:     update.GetTransaction(),
				Height: update.GetBlockHeight(),
			}
		}
	}()
	return ch, nil
}

type TxOrErr struct {
	Tx     *btcutil.Tx
	Height int32
	Err    error
}

func (c *Client) ScanRawTransactionsByHeights(ctx context.Context, startHeight, stopHeight int32, filter *btcrpc.TransactionFilter, opts ...grpc.CallOption) (<-chan TransactionOrErr, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.ScanTransactionsRequest{
		StartBlock: btcrpc.NewBlockLocatorHeight(startHeight),
		StopBlock:  btcrpc.NewBlockLocatorHeight(stopHeight),
		Filter:     filter,
	}
	str, err := c.btcd.ScanRawTransactions(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	ch := make(chan TxOrErr)
	go func() {
		for {
			update, err := str.Recv()
			if err == io.EOF {
				close(ch)
				return
			} else if err != nil {
				ch <- TxOrErr{Err: err}
				continue
			}

			tx, err := btcutil.NewTxFromBytes(update.GetTransaction())
			ch <- TransactionOrErr{
				Tx:     tx,
				Height: update.GetBlockHeight(),
				Err:    err,
			}
		}
	}()
	return ch, nil
}

func (c *Client) SubmitTransaction(ctx context.Context, tx *wire.MsgTx, opts ...grpc.CallOption) (*chainhash.Hash, error) {
	ctx = ensureCtx(ctx)

	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return nil, err
	}

	req := &btcrpc.SubmitTransactionRequest{Transaction: buf.Bytes()}
	ret, err := c.btcd.SubmitTransaction(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	return chainhash.NewHash(ret.GetHash())
}

func (c *Client) GetAddressTransactions(ctx context.Context, address string, nbSkip, nbFetch uint32, opts ...grpc.CallOption) (confirmed []*btcrpc.Transaction, unconfirmed []*btcrpc.MempoolTransaction, err error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetAddressTransactionsRequest{
		Address: address,
		NbSkip:  nbSkip,
		NbFetch: nbFetch,
	}
	ret, err := c.btcd.GetAddressTransactions(ctx, req, opts...)
	if err != nil {
		return nil, nil, err
	}

	return ret.GetConfirmedTransactions(), ret.GetUnconfirmedTransactions(), nil
}

func (c *Client) GetRawAddressTransactions(ctx context.Context, address string, nbSkip, nbFetch uint32, opts ...grpc.CallOption) (confirmed []*btcrpc.Transaction, unconfirmed []*btcrpc.MempoolTransaction, err error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetAddressTransactionsRequest{
		Address: address,
		NbSkip:  nbSkip,
		NbFetch: nbFetch,
	}
	ret, err := c.btcd.GetRawAddressTransactions(ctx, req, opts...)
	if err != nil {
		return nil, nil, err
	}

	confirmed = make([]*btcutil.Tx, len(ret.GetConfirmedTransactions()))
	for i, t := range ret.GetConfirmedTransactions() {
		confirmed[i], err = btcutil.NewTxFromBytes(t.GetSerialized())
		if err != nil {
			return nil, nil, err
		}
	}
	unconfirmed = make([]*btcutil.Tx, len(ret.GetUnconfirmedTransactions()))
	for i, t := range ret.GetUnconfirmedTransactions() {
		unconfirmed[i], err = btcutil.NewTxFromBytes(t.GetSerialized())
		if err != nil {
			return nil, nil, err
		}
	}

	return confirmed, unconfirmed, nil
}

func (c *Client) GetAddressUnspentOutputs(ctx context.Context, address string, opts ...grpc.CallOption) ([]*btcrpc.UnspentOutput, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetAddressUnspentOutputsRequest{Address: address}
	ret, err := c.btcd.GetAddressUnspentOutputs(ctx, req, opts...)
	return ret.GetOutputs(), err
}

// TransactionSubscriptionEvent is an event emitted on the channel of a
// TransactionSubscription.  If an error occurred with receiving the event, the
// Err field will be non-nil.
// Depending on the type of the event, either ConfirmedTransaction or
// AcceptedTransaction is provided.
type TransactionSubscriptionEvent struct {
	//TODO(stevenroose) use custom type here instead of the protobuf one?
	Type btcrpc.TransactionNotification_Type

	// The following two fields are set when SubscribeTransactions is used.
	ConfirmedTransaction *btcrpc.Transaction
	AcceptedTransaction  *btcrpc.MempoolTransaction
	// The following two fields are set when SubscribeRawTransactions is used.
	RawConfirmedTransaction *btcutil.Tx
	RawAcceptedTransaction  *btcutil.Tx

	Err error
}

// TransactionSubscription is used to manage an open subscription to transaction
// notifications.
type TransactionSubscription struct {
	cancel func()
	events chan TransactionSubscriptionEvent

	// strMtx is used to make sure only one call to str.Send() is made at the
	// same time.
	strMtx sync.Mutex
	str    Btcd_SubscribeTransactionsClient
}

// Events returns a channel with new events.
func (s *TransactionSubscription) Events() <-chan TransactionSubscriptionEvent {
	return s.events
}

// Cancel cancels the subscription.
func (s *TransactionSubscription) Cancel() {
	s.cancel()
}

// AddFilter adds a filter to the existing filter being used by the server.
func (s *TransactionSubscription) AddFilter(filter *TransactionFilter) error {
	s.strMtx.Lock()
	err := s.str.Send(&SubscribeTransactionsRequest{Subscribe: filter})
	s.strMtx.Unlock()
	return err
}

// RemoveFilter removes a filter from the filter being used by the server.
func (s *TransactionSubscription) RemoveFilter(filter *TransactionFilter) error {
	s.strMtx.Lock()
	err := s.str.Send(&SubscribeTransactionsRequest{Unsubscribe: filter})
	s.strMtx.Unlock()
	return err
}

func (c *Client) SubscribeTransactions(ctx context.Context, filter *TransactionFilter, includeMempool bool, opts ...grpc.CallOption) (*TransactionSubscription, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.SubscribeTransactionsRequest{
		Subscribe:      filter,
		IncludeMempool: includeMempool,
	}
	newCtx, cancel := context.WithCancel(ctx)
	str, err := c.btcd.SubscribeTransactions(newCtx, opts...)
	if err != nil {
		return nil, err
	}

	if err := str.Send(req); err != nil {
		cancel()
		return nil, err
	}

	ch := make(chan TransactionSubscriptionEvent)
	go func() {
		for {
			n, err := str.Recv()
			if err == io.EOF {
				close(ch)
				return
			} else if err != nil {
				ch <- TransactionSubscriptionEvent{Err: err}
				continue
			}

			event := TransactionSubscriptionEvent{Type: n.GetType()}
			switch n.GetType() {
			case TransactionNotification_ACCEPTED:
				t := n.GetAcceptedTransaction()
				tx, err := btcutil.NewTxFromBytes(t.GetTransaction().GetSerialized())
				if err != nil {
					event.Err = err
					ch <- event
					continue
				}
				event.AcceptedTransaction = &mempool.TxDesc{
					TxDesc: mining.TxDesc{
						Tx:       tx,
						Added:    time.Unix(t.GetAddedTime(), 0),
						Height:   t.GetHeight(),
						Fee:      t.GetFee(),
						FeePerKB: t.GetFeePerByte() * 1000,
					},
					StartingPriority: t.GetStartingPriority(),
				}
			case TransactionNotification_CONFIRMED:
				t := n.GetConfirmedTransaction()
				tx, err := btcutil.NewTxFromBytes(t.GetSerialized())
				if err != nil {
					event.Err = err
					ch <- event
					continue
				}
				event.ConfirmedTransaction = tx
			}

			ch <- event
		}
	}()

	return &TransactionSubscription{
		cancel: cancel,
		events: ch,
		str:    str,
	}, nil
}

// BlockSubscriptionEvent is an event emitted on the channel of a
// BlockSubscription.  If an error occurred with receiving the event, the
// Err field will be non-nil.
type BlockSubscriptionEvent struct {
	Err error

	Type   BlockNotification_Type
	Hash   *chainhash.Hash
	Height int32
	Header *wire.BlockHeader
}

// BlockSubscription is used to manage an open subscription to block
// notifications.
type BlockSubscription struct {
	cancel func()
	events chan BlockSubscriptionEvent
}

// Events returns a channel with new events.
func (s *BlockSubscription) Events() <-chan BlockSubscriptionEvent {
	return s.events
}

// Close closes the subscription.
func (s *BlockSubscription) Close() {
	s.cancel()
}

func (c *Client) SubscribeBlocks(ctx context.Context, opts ...grpc.CallOption) (*BlockSubscription, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.SubscribeBlocksRequest{}
	newCtx, cancel := context.WithCancel(ctx)
	str, err := c.btcd.SubscribeBlocks(newCtx, req, opts...)
	if err != nil {
		return nil, err
	}

	ch := make(chan BlockSubscriptionEvent)
	go func() {
		for {
			n, err := str.Recv()
			if err == io.EOF {
				close(ch)
				return
			} else if err != nil {
				ch <- BlockSubscriptionEvent{Err: err}
				continue
			}

			blockInfo := n.GetBlock()
			hash, err := chainhash.NewHash(blockInfo.GetHash())
			if err != nil {
				ch <- BlockSubscriptionEvent{Err: err}
				continue
			}
			prevBlock, err := chainhash.NewHash(blockInfo.GetPreviousBlock())
			if err != nil {
				ch <- BlockSubscriptionEvent{Err: err}
				continue
			}
			merkleRoot, err := chainhash.NewHash(blockInfo.GetMerkleRoot())
			if err != nil {
				ch <- BlockSubscriptionEvent{Err: err}
				continue
			}
			header := &wire.BlockHeader{
				Version:    blockInfo.GetVersion(),
				PrevBlock:  *prevBlock,
				MerkleRoot: *merkleRoot,
				Timestamp:  time.Unix(blockInfo.GetTime(), 0),
				Bits:       blockInfo.GetBits(),
				Nonce:      blockInfo.GetNonce(),
			}
			ch <- BlockSubscriptionEvent{
				Type:   n.GetType(),
				Hash:   hash,
				Height: blockInfo.GetHeight(),
				Header: header,
			}
		}
	}()

	return &BlockSubscription{
		cancel: cancel,
		events: ch,
	}, nil
}

func (c *Client) GetPeers(ctx context.Context, permanent bool, opts ...grpc.CallOption) ([]*btcrpc.GetPeersResponse_PeerInfo, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetPeersRequest{Permanent: permanent}
	ret, err := c.btcd.GetPeers(ctx, req, opts...)
	return ret.GetPeers(), err
}

func (c *Client) ConnectPeer(ctx context.Context, peerAddress string, permanent bool, opts ...grpc.CallOption) error {
	ctx = ensureCtx(ctx)

	req := &btcrpc.ConnectPeerRequest{
		PeerAddress: peerAddress,
		Permanent:   permanent,
	}
	_, err := c.btcd.ConnectPeer(ctx, req, opts...)
	return err
}

func (c *Client) DisconnectPeerByAddress(ctx context.Context, peerAddress string, permanent bool, opts ...grpc.CallOption) error {
	ctx = ensureCtx(ctx)

	req := &btcrpc.DisconnectPeerRequest{
		Peer: &btcrpc.DisconnectPeerRequest_PeerAddress{
			PeerAddress: peerAddress,
		},
		Permanent: permanent,
	}
	_, err := c.btcd.DisconnectPeer(ctx, req, opts...)
	return err
}

func (c *Client) DisconnectPeerByID(ctx context.Context, peerID int32, permanent bool, opts ...grpc.CallOption) error {
	ctx = ensureCtx(ctx)

	req := &btcrpc.DisconnectPeerRequest{
		Peer: &btcrpc.DisconnectPeerRequest_PeerId{
			PeerId: peerID,
		},
		Permanent: permanent,
	}
	_, err := c.btcd.DisconnectPeer(ctx, req, opts...)
	return err
}

func (c *Client) GetMiningInfo(ctx context.Context, opts ...grpc.CallOption) (*btcrpc.GetMiningInfoResponse, error) {
	ctx = ensureCtx(ctx)

	req := &btcrpc.GetMiningInfoRequest{}
	return c.btcd.GetMiningInfo(ctx, req, opts...)
}
