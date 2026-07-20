package mempool

import (
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
)

// NotificationType represents the type of a notification message.
type NotificationType int

// NotificationCallback is used for a caller to provide a callback for
// notifications about various mempool events.
type NotificationCallback func(*Notification)

// Constants for the type of a notification message.
const (
	NTTxAccepted NotificationType = iota

	NTTxRemoved
)

// notificationTypeStrings is a map of notification types back to their constant
// names for pretty printing.
var notificationTypeStrings = map[NotificationType]string{
	NTTxAccepted: "NTTxAccepted",
	NTTxRemoved:  "NTTxRemoved",
}

type NTTxAcceptedData struct {
	Tx       *btcutil.Tx
	UtxoView *blockchain.UtxoViewpoint
}

// Notification defines notification that is sent to the caller via the callback
// function provided during the call to New and consists of a notification type
// as well as associated data that depends on the type as follows:
//   - NTTxAccepted:   *NTTxAcceptedData
//   - NTTxRemoved :   *chainhash.Hash
type Notification struct {
	Type NotificationType
	Data interface{}
}

func (mp *TxPool) Subscribe(callback NotificationCallback) {
	mp.notificationsLock.Lock()
	mp.notifications = append(mp.notifications, callback)
	mp.notificationsLock.Unlock()
}

func (mp *TxPool) sendNotification(typ NotificationType, data interface{}) {
	// Generate and send the notification.
	n := Notification{Type: typ, Data: data}
	mp.notificationsLock.RLock()
	for _, callback := range mp.notifications {
		callback(&n)
	}
	mp.notificationsLock.RUnlock()
}
