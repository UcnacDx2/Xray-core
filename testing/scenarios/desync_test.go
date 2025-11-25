package scenarios

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/proxy"

	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/proxyman"
	_ "github.com/xtls/xray-core/app/proxyman/inbound"
	_ "github.com/xtls/xray-core/app/proxyman/outbound"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/proxy/freedom"
	"github.com/xtls/xray-core/proxy/socks"
	"github.com/xtls/xray-core/transport/internet"
)

// TestDesyncFreedomGoogle tests the desync feature with a real Google server.
// This test requires root privileges to run, as it uses raw sockets.
// To run this test, use the following command:
// sudo go test -v ./testing/scenarios/... -run TestDesyncFreedomGoogle
func TestDesyncFreedomGoogle(t *testing.T) {
	config := &core.Config{
		Inbound: []*core.InboundHandlerConfig{
			{
				Tag: "socks-in",
				ReceiverSettings: serial.ToTypedMessage(&proxyman.ReceiverConfig{
					PortList: &net.PortList{Range: []*net.PortRange{{From: 10808, To: 10808}}},
					Listen:    &net.IPOrDomain{Address: &net.IPOrDomain_Ip{Ip: []byte{127, 0, 0, 1}}},
				}),
				ProxySettings: serial.ToTypedMessage(&socks.ServerConfig{
					AuthType: socks.AuthType_NO_AUTH,
					UdpEnabled: true,
				}),
			},
		},
		Outbound: []*core.OutboundHandlerConfig{
			{
				Tag: "fragment-out",
				ProxySettings: serial.ToTypedMessage(&freedom.Config{
					DomainStrategy: internet.DomainStrategy_USE_IP,
				}),
				SenderSettings: serial.ToTypedMessage(&proxyman.SenderConfig{
					StreamSettings: &internet.StreamConfig{
						SocketSettings: &internet.SocketConfig{
							TcpKeepAliveInterval: 100,
							Mark:                 255,
							Desync: &internet.DesyncConfig{
								Enabled: true,
								Ttl:     2,
								Payload: []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"),
							},
						},
					},
				}),
			},
		},
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&proxyman.InboundConfig{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
		},
	}

	instance, err := core.New(config)
	assert.NoError(t, err)

	err = instance.Start()
	assert.NoError(t, err)
	defer instance.Close()

	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:10808", nil, proxy.Direct)
	assert.NoError(t, err)

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		},
		Timeout: 30 * time.Second,
	}

	resp, err := httpClient.Get("https://www.google.com")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
