package metrics

import (
	"github.com/chia-network/go-chia-libs/pkg/types"
	log "github.com/sirupsen/logrus"
)

// OpenWebsocket sets up the RPC client and subscribes to relevant topics
func (m *Metrics) OpenWebsocket() error {
	err := m.websocketClient.SubscribeSelf()
	if err != nil {
		return err
	}

	err = m.websocketClient.Subscribe("metrics")
	if err != nil {
		return err
	}

	err = m.websocketClient.AddHandler(m.websocketReceive)
	if err != nil {
		return err
	}

	m.websocketClient.AddDisconnectHandler(m.disconnectHandler)
	m.websocketClient.AddReconnectHandler(m.reconnectHandler)

	return nil
}

// CloseWebsocket closes the websocket connection
func (m *Metrics) CloseWebsocket() error {
	// @TODO reenable once fixed in the upstream dep
	//return m.websocketClient.DaemonService.CloseConnection()
	return nil
}

func (m *Metrics) websocketReceive(resp *types.WebsocketResponse, err error) {
	if err != nil {
		log.Errorf("Websocket received err: %s\n", err.Error())
		return
	}

	log.Printf("recv: %s %s\n", resp.Origin, resp.Command)
	log.Debugf("origin: %s command: %s destination: %s data: %s\n", resp.Origin, resp.Command, resp.Destination, string(resp.Data))

	switch resp.Command {
	case "block":
		m.receiveBlock(resp)
	}
}

func (m *Metrics) disconnectHandler() {
	log.Debug("Calling disconnect handlers")
}

func (m *Metrics) reconnectHandler() {
	log.Debug("Calling reconnect handlers")
}
