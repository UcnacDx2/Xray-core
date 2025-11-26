package scenarios

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/proxy"

	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/dns"
	"github.com/xtls/xray-core/app/proxyman"
	_ "github.com/xtls/xray-core/app/proxyman/inbound"
	_ "github.com/xtls/xray-core/app/proxyman/outbound"
	"github.com/xtls/xray-core/app/router"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/proxy/freedom"
	"github.com/xtls/xray-core/proxy/socks"
	"github.com/xtls/xray-core/transport/internet"
)

func TestDesyncFreedomThreads(t *testing.T) {
	config := &core.Config{
		Inbound: []*core.InboundHandlerConfig{
			{
				Tag: "socks-in-desync",
				ReceiverSettings: serial.ToTypedMessage(&proxyman.ReceiverConfig{
					PortList: &net.PortList{Range: []*net.PortRange{{From: 10808, To: 10808}}},
					Listen:    &net.IPOrDomain{Address: &net.IPOrDomain_Ip{Ip: []byte{127, 0, 0, 1}}},
				}),
				ProxySettings: serial.ToTypedMessage(&socks.ServerConfig{
					AuthType: socks.AuthType_NO_AUTH,
					UdpEnabled: true,
				}),
			},
			{
				Tag: "socks-in-no-desync",
				ReceiverSettings: serial.ToTypedMessage(&proxyman.ReceiverConfig{
					PortList: &net.PortList{Range: []*net.PortRange{{From: 10809, To: 10809}}},
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
							Desync: &internet.DesyncConfig{
								Enabled: true,
								Ttl:     2,
								Payload: []byte("GET / HTTP/1.1\r\nHost: www.baidu.com\r\n\r\n"),
								Delay:   10,
							},
						},
					},
				}),
			},
			{
				Tag: "direct-out",
				ProxySettings: serial.ToTypedMessage(&freedom.Config{
					DomainStrategy: internet.DomainStrategy_USE_IP,
				}),
			},
		},
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(&dns.Config{
				NameServer: []*dns.NameServer{
					{
						Address: &net.Endpoint{
							Network: net.Network_UDP,
							Address: &net.IPOrDomain{
								Address: &net.IPOrDomain_Domain{
									Domain: "https://77.88.8.8/dns-query",
								},
							},
						},
					},
				},
			}),
			serial.ToTypedMessage(&router.Config{
				Rule: []*router.RoutingRule{
					{
						InboundTag: []string{"socks-in-desync"},
						TargetTag: &router.RoutingRule_Tag{
							Tag: "fragment-out",
						},
					},
					{
						InboundTag: []string{"socks-in-no-desync"},
						TargetTag: &router.RoutingRule_Tag{
							Tag: "direct-out",
						},
					},
					{
						TargetTag: &router.RoutingRule_Tag{
							Tag: "direct-out",
						},
						Geoip: []*router.GeoIP{
							{
								Cidr: []*router.CIDR{
									{
										Ip:     []byte{77, 88, 8, 8},
										Prefix: 32,
									},
								},
							},
						},
					},
				},
			}),
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

	dialerDesync, err := proxy.SOCKS5("tcp", "127.0.0.1:10808", nil, proxy.Direct)
	assert.NoError(t, err)

	dialerNoDesync, err := proxy.SOCKS5("tcp", "127.0.0.1:10809", nil, proxy.Direct)
	assert.NoError(t, err)

	httpClientDesync := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialerDesync.Dial(network, addr)
			},
		},
		Timeout: 30 * time.Second,
	}

	httpClientNoDesync := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialerNoDesync.Dial(network, addr)
			},
		},
		Timeout: 30 * time.Second,
	}

	urls := []string{
		"https://threads.net",
		"https://v2ex.com",
		"https://linux.do",
	}

	var lastErr error
	success := false
	for _, url := range urls {
		_, err := httpClientDesync.Get(url)
		if err == nil {
			success = true
			continue
		}

		_, err = httpClientNoDesync.Get(url)
		if err == nil {
			success = true
			continue
		}
		lastErr = err
	}

	assert.True(t, success, lastErr)
}
