package btcrpc

import (
	"bytes"
	"context"
	fmt "fmt"
	"io"
	"sync"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/mining"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	grpc "google.golang.org/grpc"
)

// WrappedBtcdClient is a utility wrapper around BtcdClient.
// It allows calling methods using Go types for arguments and converts return
// vallues into Go types as well.
//
// For the documentation of the methods, we refer to the gRPC specification file
// which can also be found on the BtcdClient interface.
//
// In order to retrieve the underlying BtcdClient, use the Client method.
type WrappedBtcdClient btcdClient

// NewWrappedBtcdClient creates a new gRPC client for btcd that can be used with
// Go struct types instead of the gRPC definition types.
func NewWrappedBtcdClient(grpcConn *grpc.ClientConn) *WrappedBtcdClient {
	return (*WrappedBtcdClient)(&btcdClient{grpcConn})
}

func (c *WrappedBtcdClient) client() BtcdClient {
	return (*btcdClient)(c)
}

func (c *WrappedBtcdClient) GetSystemInfo(ctx context.Context, opts ...grpc.CallOption) (*GetSystemInfoResponse, error) {
	req := &GetSystemInfoRequest{}
	return c.client().GetSystemInfo(ctx, req, opts...)
}

func (c *WrappedBtcdClient) SetDebugLevel(ctx context.Context, level, subsystem string, opts ...grpc.CallOption) error {
	lvl, found := SetDebugLevelRequest_DebugLevel_value[level]
	if !found {
		return fmt.Errorf("Debug level '%v' does not exist", level)
	}
	ss, found := SetDebugLevelRequest_Subsystem_value[subsystem]
	if !found {
		return fmt.Errorf("Subsystem '%v' does not exist", subsystem)
	}

	req := &SetDebugLevelRequest{
		Level:     SetDebugLevelRequest_DebugLevel(lvl),
		Subsystem: SetDebugLevelRequest_Subsystem(ss),
	}
	_, err := c.client().SetDebugLevel(ctx, req, opts...)
	return err
}

func (c *WrappedBtcdClient) StopDaemon(ctx context.Context, opts ...grpc.CallOption) error {
	req := &StopDaemonRequest{}
	_, err := c.client().StopDaemon(ctx, req, opts...)
	return err
}

func (c *WrappedBtcdClient) GetNetworkInfo(ctx context.Context, opts ...grpc.CallOption) (*GetNetworkInfoResponse, error) {
	req := &GetNetworkInfoRequest{}
	return c.client().GetNetworkInfo(ctx, req, opts...)
}

func (c *WrappedBtcdClient) GetNetworkHashRate(ctx context.Context, endHeight, nbBlocks int32, opts ...grpc.CallOption) (uint64, error) {
	req := &GetNetworkHashRateRequest{
		EndHeight: endHeight,
		NbBlocks:  nbBlocks,
	}
	ret, err := c.client().GetNetworkHashRate(ctx, req, opts...)
	return ret.GetHashrate(), err
}

func (c *WrappedBtcdClient) GetMempoolInfo(ctx context.Context, opts ...grpc.CallOption) (*GetMempoolInfoResponse, error) {
	req := &GetMempoolInfoRequest{}
	return c.client().GetMempoolInfo(ctx, req, opts...)
}

func (c *WrappedBtcdClient) GetMempoolHashes(ctx context.Context, opts ...grpc.CallOption) ([]*chainhash.Hash, error) {
	req := &GetMempoolRequest{FullTransactions: false}
	ret, err := c.client().GetMempool(ctx, req, opts...)
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

func (c *WrappedBtcdClient) GetMempoolTxs(ctx context.Context, opts ...grpc.CallOption) ([]*mempool.TxDesc, error) {
	req := &GetMempoolRequest{FullTransactions: true}
	ret, err := c.client().GetMempool(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	txDescs := make([]*mempool.TxDesc, len(ret.GetTransactions()))
	for i, t := range ret.GetTransactions() {
		tx, err := btcutil.NewTxFromBytes(t.GetTransaction().GetSerialized())
		if err != nil {
			return nil, err
		}
		txDescs[i] = &mempool.TxDesc{
			TxDesc: mining.TxDesc{
				Tx:       tx,
				Added:    time.Unix(t.GetAddedTime(), 0),
				Height:   t.GetHeight(),
				Fee:      t.GetFee(),
				FeePerKB: t.GetFeePerByte() * 1000,
			},
			StartingPriority: t.GetStartingPriority(),
		}
	}
	return txDescs, nil
}

func (c *WrappedBtcdClient) GetRawMempool(ctx context.Context, opts ...grpc.CallOption) ([]*btcutil.Tx, error) {
	req := &GetRawMempoolRequest{}
	ret, err := c.client().GetRawMempool(ctx, req, opts...)
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

func (c *WrappedBtcdClient) GetBestBlockInfo(ctx context.Context, opts ...grpc.CallOption) (*BlockInfo, error) {
	req := &GetBestBlockInfoRequest{}
	ret, err := c.client().GetBestBlockInfo(ctx, req, opts...)
	return ret.GetInfo(), err
}

func (c *WrappedBtcdClient) GetBlockInfoByHash(ctx context.Context, hash *chainhash.Hash, opts ...grpc.CallOption) (*BlockInfo, error) {
	req := &GetBlockInfoRequest{Locator: NewBlockLocatorHash(hash)}
	ret, err := c.client().GetBlockInfo(ctx, req, opts...)
	return ret.GetInfo(), err
}

func (c *WrappedBtcdClient) GetBlockInfoByHeight(ctx context.Context, height int32, opts ...grpc.CallOption) (*BlockInfo, error) {
	req := &GetBlockInfoRequest{Locator: NewBlockLocatorHeight(height)}
	ret, err := c.client().GetBlockInfo(ctx, req, opts...)
	return ret.GetInfo(), err
}

func (c *WrappedBtcdClient) GetBlockByHash(ctx context.Context, hash *chainhash.Hash, opts ...grpc.CallOption) (*Block, error) {
	req := &GetBlockRequest{Locator: NewBlockLocatorHash(hash)}
	ret, err := c.client().GetBlock(ctx, req, opts...)
	return ret.GetBlock(), err
}

func (c *WrappedBtcdClient) GetBlockByHeight(ctx context.Context, height int32, opts ...grpc.CallOption) (*Block, error) {
	req := &GetBlockRequest{Locator: NewBlockLocatorHeight(height)}
	ret, err := c.client().GetBlock(ctx, req, opts...)
	return ret.GetBlock(), err
}

func (c *WrappedBtcdClient) GetRawBlockByHash(ctx context.Context, hash *chainhash.Hash, opts ...grpc.CallOption) (*btcutil.Block, error) {
	req := &GetRawBlockRequest{Locator: NewBlockLocatorHash(hash)}
	ret, err := c.client().GetRawBlock(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	return btcutil.NewBlockFromBytes(ret.GetBlock())
}

func (c *WrappedBtcdClient) GetRawBlockByHeight(ctx context.Context, height int32, opts ...grpc.CallOption) (*btcutil.Block, error) {
	req := &GetRawBlockRequest{Locator: NewBlockLocatorHeight(height)}
	ret, err := c.client().GetRawBlock(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	return btcutil.NewBlockFromBytes(ret.GetBlock())
}

func (c *WrappedBtcdClient) SubmitBlock(ctx context.Context, block *wire.MsgBlock, opts ...grpc.CallOption) (*chainhash.Hash, error) {
	var buf bytes.Buffer
	if err := block.Serialize(&buf); err != nil {
		return nil, err
	}

	req := &SubmitBlockRequest{Block: buf.Bytes()}
	ret, err := c.client().SubmitBlock(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	return chainhash.NewHash(ret.GetHash())
}

func (c *WrappedBtcdClient) GetTransaction(ctx context.Context, txHash *chainhash.Hash, opts ...grpc.CallOption) (*Transaction, error) {
	req := &GetTransactionRequest{Hash: txHash[:]}
	ret, err := c.client().GetTransaction(ctx, req, opts...)
	return ret.GetTransaction(), err
}

func (c *WrappedBtcdClient) GetRawTransaction(ctx context.Context, txHash *chainhash.Hash, opts ...grpc.CallOption) (*btcutil.Tx, error) {
	req := &GetRawTransactionRequest{Hash: txHash[:]}
	ret, err := c.client().GetRawTransaction(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	return btcutil.NewTxFromBytes(ret.GetTransaction())
}

type TxOrErr struct {
	Tx  *btcutil.Tx
	Err error
}

func (c *WrappedBtcdClient) RescanTransactionsByHeights(ctx context.Context, startHeight, stopHeight int32, filter *TransactionFilter, opts ...grpc.CallOption) (<-chan TxOrErr, error) {
	req := &RescanTransactionsRequest{
		StartBlock: NewBlockLocatorHeight(startHeight),
		StopBlock:  NewBlockLocatorHeight(stopHeight),
		Filter:     filter,
	}
	str, err := c.client().RescanTransactions(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	ch := make(chan TxOrErr)
	go func() {
		for {
			tx, err := str.Recv()
			if err == io.EOF {
				return
			} else if err != nil {
				ch <- TxOrErr{Err: err}
				continue
			}

			t, err := btcutil.NewTxFromBytes(tx.GetSerialized())
			ch <- TxOrErr{t, err}
		}
	}()
	return ch, nil
}

func (c *WrappedBtcdClient) SubmitTransaction(ctx context.Context, tx *wire.MsgTx, opts ...grpc.CallOption) (*chainhash.Hash, error) {
	var buf bytes.Buffer
	if err := tx.Serialize(&buf); err != nil {
		return nil, err
	}

	req := &SubmitTransactionRequest{Transaction: buf.Bytes()}
	ret, err := c.client().SubmitTransaction(ctx, req, opts...)
	if err != nil {
		return nil, err
	}

	return chainhash.NewHash(ret.GetHash())
}

func (c *WrappedBtcdClient) GetAddressTransactions(ctx context.Context, address string, nbSkip, nbFetch uint32, opts ...grpc.CallOption) (confirmed []*btcutil.Tx, unconfirmed []*btcutil.Tx, err error) {
	req := &GetAddressTransactionsRequest{
		Address: address,
		NbSkip:  nbSkip,
		NbFetch: nbFetch,
	}
	ret, err := c.client().GetAddressTransactions(ctx, req, opts...)
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

func (c *WrappedBtcdClient) GetAddressUnspentOutputs(ctx context.Context, address string, opts ...grpc.CallOption) ([]*UnspentOutput, error) {
	req := &GetAddressUnspentOutputsRequest{Address: address}
	ret, err := c.client().GetAddressUnspentOutputs(ctx, req, opts...)
	return ret.GetOutputs(), err
}

// TransactionSubscriptionEvent is an event emitted on the channel of a
// TransactionSubscription.  If an error occurred with receiving the event, the
// Err field will be non-nil.
// Depending on the type of the event, either ConfirmedTransaction or
// UnconfirmedTransaction is provided.
type TransactionSubscriptionEvent struct {
	Err error

	Type                 TransactionNotification_Type
	ConfirmedTransaction *btcutil.Tx
	AcceptedTransaction  *mempool.TxDesc
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

// Close closes the subscription.
func (s *TransactionSubscription) Close() {
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

func (c *WrappedBtcdClient) SubscribeTransactions(ctx context.Context, filter *TransactionFilter, includeMempool bool, opts ...grpc.CallOption) (*TransactionSubscription, error) {
	req := &SubscribeTransactionsRequest{
		Subscribe:      filter,
		IncludeMempool: includeMempool,
	}
	newCtx, cancel := context.WithCancel(ctx)
	str, err := c.client().SubscribeTransactions(newCtx, opts...)
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

func (c *WrappedBtcdClient) SubscribeBlocks(ctx context.Context, opts ...grpc.CallOption) (*BlockSubscription, error) {
	req := &SubscribeBlocksRequest{}
	newCtx, cancel := context.WithCancel(ctx)
	str, err := c.client().SubscribeBlocks(newCtx, req, opts...)
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

func (c *WrappedBtcdClient) GetPeers(ctx context.Context, permanent bool, opts ...grpc.CallOption) ([]*GetPeersResponse_PeerInfo, error) {
	req := &GetPeersRequest{Permanent: permanent}
	ret, err := c.client().GetPeers(ctx, req, opts...)
	return ret.GetPeers(), err
}

func (c *WrappedBtcdClient) ConnectPeer(ctx context.Context, peerAddress string, permanent bool, opts ...grpc.CallOption) error {
	req := &ConnectPeerRequest{
		PeerAddress: peerAddress,
		Permanent:   permanent,
	}
	_, err := c.client().ConnectPeer(ctx, req, opts...)
	return err
}

func (c *WrappedBtcdClient) DisconnectPeerByAddress(ctx context.Context, peerAddress string, permanent bool, opts ...grpc.CallOption) error {
	req := &DisconnectPeerRequest{
		Peer: &DisconnectPeerRequest_PeerAddress{
			PeerAddress: peerAddress,
		},
		Permanent: permanent,
	}
	_, err := c.client().DisconnectPeer(ctx, req, opts...)
	return err
}

func (c *WrappedBtcdClient) DisconnectPeerByID(ctx context.Context, peerID int32, permanent bool, opts ...grpc.CallOption) error {
	req := &DisconnectPeerRequest{
		Peer: &DisconnectPeerRequest_PeerId{
			PeerId: peerID,
		},
		Permanent: permanent,
	}
	_, err := c.client().DisconnectPeer(ctx, req, opts...)
	return err
}

func (c *WrappedBtcdClient) GetMiningInfo(ctx context.Context, opts ...grpc.CallOption) (*GetMiningInfoResponse, error) {
	req := &GetMiningInfoRequest{}
	return c.client().GetMiningInfo(ctx, req, opts...)
}
