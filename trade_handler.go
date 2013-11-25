package mtgox

import (
	"encoding/json"
	"strconv"
	"time"
)

type TradePayload struct {
	StreamHeader
	Trade Trade `json:"trade"`
}

type Trade struct {
	Type       string
	Tid        string
	Amount     int64
	Price      int64
	Instrument string
	Currency   string
	TradeType  string
	Primary    string
	Properties string
	Timestamp  time.Time
}

func (t *Trade) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}

	err := json.Unmarshal(data, &raw)
	if err != nil {
		return err
	}

	for k, v := range raw {
		switch vv := v.(type) {
		case string:
			switch k {
			case "type":
				t.Type = vv
			case "tid":
				t.Tid = vv
			case "item":
				t.Instrument = vv
			case "price_currency":
				t.Currency = vv
			case "trade_type":
				t.TradeType = vv
			case "primary":
				t.Primary = vv
			case "properties":
				t.Properties = vv
			case "amount_int":
				t.Amount, err = strconv.ParseInt(vv, 10, 64)
				if err != nil {
					return err
				}
			case "price_int":
				t.Price, err = strconv.ParseInt(vv, 10, 64)
				if err != nil {
					return err
				}
			}

		case float64:
			switch k {
			case "date":
				t.Timestamp = time.Unix(int64(vv), 0)
			}
		}
	}

	return nil
}
func (g *Client) handleTrade(data []byte) {
	var payload TradePayload
	err := json.Unmarshal(data, &payload)
	if err != nil {
		select {
		case g.Errors <- err:
		default:
		}
	}
	select {
	case g.Trades <- &payload:
	default:
	}
}
