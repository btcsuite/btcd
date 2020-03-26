package btcjson

import (
	"encoding/json"
	"net/url"
)

// GetZmqNotificationResult models the data returned from the getzmqnotifications command.
type GetZmqNotificationResult []struct {
	Type          string   // Type of notification
	Address       *url.URL // Address of the publisher
	HighWaterMark int      // Outbound message high water mark
}

func (z *GetZmqNotificationResult) MarshalJSON() ([]byte, error) {
	var out []map[string]interface{}
	for _, notif := range *z {
		out = append(out,
			map[string]interface{}{
				"type":    notif.Type,
				"address": notif.Address.String(),
				"hwm":     notif.HighWaterMark,
			})
	}
	return json.Marshal(out)
}

// UnmarshalJSON satisfies the json.Unmarshaller interface
func (z *GetZmqNotificationResult) UnmarshalJSON(bytes []byte) error {
	type basicNotification struct {
		Type    string
		Address string
		Hwm     int
	}

	var basics []basicNotification
	if err := json.Unmarshal(bytes, &basics); err != nil {
		return err
	}

	var notifications GetZmqNotificationResult
	for _, basic := range basics {

		address, err := url.Parse(basic.Address)
		if err != nil {
			return err
		}

		notifications = append(notifications, struct {
			Type          string
			Address       *url.URL
			HighWaterMark int
		}{
			Type:          basic.Type,
			Address:       address,
			HighWaterMark: basic.Hwm,
		})
	}

	*z = notifications

	return nil
}
