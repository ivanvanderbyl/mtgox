package gox

/*
  Package gox provides a streaming implementation of Mt. Gox's bitcoin trading API
  built on the Gorilla Websocket library
*/

import (
	"crypto/hmac"
	"crypto/sha512"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type StreamType string
type OrderType string

const (
	secureApiHost string = "wss://websocket.mtgox.com:443"
	apiHost       string = "ws://websocket.mtgox.com:80"
	apiPath       string = "/mtgox"
	httpEndpoint  string = "http://mtgox.com/api/2"
	originUrl     string = "http://websocket.mtgox.com"

	BID OrderType = "bid"
	ASK OrderType = "ask"

	secureConn bool = false

	BitcoinDivision = 1e8
)

type Gox struct {
	key    []byte
	secret []byte
	conn   *websocket.Conn

	Ticker chan *TickerPayload
	Info   chan *Info
	Depth  chan *DepthPayload
	Trades chan *TradePayload
	Orders chan []Order
	Errors chan error
	done   chan bool
}

type Config struct {
	Currencies []string
	Key        string
	Secret     string
	SecureConn bool
}

type payload struct {
	messageType int
	data        []byte
}

type StreamHeader struct {
	Channel     string `json:"channel"`
	ChannelName string `json:"channel_name"`
	Op          string `json:"op"`
	Origin      string `json:"origin"`
	Private     string `json:"private"`
}

func New(key, secret string, currencies ...string) (*Gox, error) {
	var mtgoxUrl string
	if secureConn {
		mtgoxUrl = fmt.Sprintf("%s%s?Currency=%s", secureApiHost, apiPath, strings.Join(currencies, ","))
	} else {
		mtgoxUrl = fmt.Sprintf("%s%s?Currency=%s", apiHost, apiPath, strings.Join(currencies, ","))
	}

	u, err := url.Parse(mtgoxUrl)
	if err != nil {
		return nil, fmt.Errorf("Error parsing URL: %s", err.Error())
	}

	var netConn net.Conn

	if secureConn {
		netConn, err = tls.Dial("tcp", u.Host, nil)
	} else {
		netConn, err = net.Dial("tcp", u.Host)
	}

	if err != nil {
		return nil, fmt.Errorf("Error connecting: %s", err.Error())
	}

	conn, _, err := websocket.NewClient(netConn, u, http.Header{"Origin": {originUrl}}, 256, 256)
	if err != nil {
		return nil, fmt.Errorf("Opening websocket: %v", err)
	}

	return NewWithConnection(key, secret, conn)
}

// Constructs a new client using an existing connection, useful for testing
func NewWithConnection(key, secret string, conn *websocket.Conn) (g *Gox, err error) {
	g = &Gox{
		conn:   conn,
		Ticker: make(chan *TickerPayload, 1),
		Info:   make(chan *Info, 1),
		Depth:  make(chan *DepthPayload, 1),
		Trades: make(chan *TradePayload, 1),
		Orders: make(chan []Order, 1),
		Errors: make(chan error, 10),
		done:   make(chan bool, 1),
	}

	g.key, err = hex.DecodeString(strings.Replace(key, "-", "", -1))
	if err != nil {
		return nil, err
	}

	g.secret, err = base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return nil, err
	}

	return g, nil
}

func (g *Gox) Start() {
	go func() {
		for p := range g.messages() {
			g.handle(p)
		}
	}()
}

func (g *Gox) Close() {
	g.done <- true
}

// Returns the raw websocket connection
func (g *Gox) Conn() *websocket.Conn {
	return g.conn
}

// Reads messages into a channel so we can select on them later
func (g *Gox) messages() <-chan []byte {
	msgs := make(chan []byte, 10)

	go func(msgs chan []byte) {
		for {
			messageType, data, err := g.conn.ReadMessage()
			if err != nil {
				g.Errors <- err
				break
			}

			if messageType == websocket.TextMessage {
				msgs <- data
			} else {
				g.Errors <- fmt.Errorf("Received unknown message type: %d", messageType)
			}
		}
	}(msgs)

	return msgs
}

func (g *Gox) sign(body []byte) ([]byte, error) {
	mac := hmac.New(sha512.New, g.secret)
	_, err := mac.Write(body)
	if err != nil {
		return nil, err
	}

	return mac.Sum(nil), nil
}

func (g *Gox) authenticatedSend(msg map[string]interface{}) error {
	if g.key == nil || g.secret == nil {
		return errors.New("API Key or secret is invalid or missing.")
	}

	req, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	signedReq, err := g.sign(req)
	if err != nil {
		return err
	}

	requestId := msg["id"]

	fullReq := append(append(g.key, signedReq...), req...)
	encodedReq := base64.StdEncoding.EncodeToString(fullReq)

	return g.conn.WriteJSON(map[string]interface{}{
		"op":      "call",
		"id":      requestId,
		"call":    encodedReq,
		"context": "mtgox.com",
	})
}

// Handler function for processing responses from mtgox
func (g *Gox) handle(data []byte) {
	var header StreamHeader
	json.Unmarshal(data, &header)

	switch header.Private {
	case "debug":
		g.handleDebug(data)

	case "ticker":
		g.handleTicker(data)

	case "trade":
		g.handleTrade(data)

	case "depth":
		g.handleDepth(data)

	case "result":
		g.handleResult(data)

	default:
		fmt.Println(header.Private)

		var payload map[string]interface{}
		json.Unmarshal(data, &payload)
		fmt.Println(string(PrettyPrintJson(payload)))

	}
}

func PrettyPrintJson(p interface{}) []byte {
	formattedJson, err := json.MarshalIndent(&p, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return formattedJson
}

func (g *Gox) call(endpoint string, params map[string]interface{}) error {
	if params == nil {
		params = make(map[string]interface{})
	}

	msg := map[string]interface{}{
		"call":   endpoint,
		"item":   "BTC",
		"params": params,
		"id":     <-ids,
		"nonce":  <-nonces,
	}

	return g.authenticatedSend(msg)
}
