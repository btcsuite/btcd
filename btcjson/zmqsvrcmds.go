package btcjson

// GetZmqNotificationsCmd defines the getzmqnotifications JSON-RPC command.
type GetZmqNotificationsCmd struct{}

// NewGetZmqNotificationsCmd returns a new instance which can be used to issue a
// getzmqnotifications JSON-RPC command.
func NewGetZmqNotificationsCmd() *GetZmqNotificationsCmd {
	return &GetZmqNotificationsCmd{}
}

func init() {
	flags := UsageFlag(0)

	MustRegisterCmd("getzmqnotifications", (*GetZmqNotificationsCmd)(nil), flags)
}
