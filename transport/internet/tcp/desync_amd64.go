//go:build linux && amd64
// +build linux,amd64

package tcp

import (
	"net"
	"time"

	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/transport/internet"
	"golang.org/x/sys/unix"
)

func performDesync(conn net.Conn, config *internet.DesyncConfig) error {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return errors.New("not a TCP connection")
	}

	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return errors.New("failed to get raw connection").Base(err)
	}

	var ttl int
	var ttlErr error
	var fd int

	err = rawConn.Control(func(rawFd uintptr) {
		fd = int(rawFd)
		ttl, ttlErr = unix.GetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_TTL)
	})

	if err != nil {
		return errors.New("failed to get raw connection control").Base(err)
	}
	if ttlErr != nil {
		return errors.New("failed to get IP_TTL").Base(ttlErr)
	}

	defer rawConn.Control(func(rawFd uintptr) {
		unix.SetsockoptInt(int(rawFd), unix.IPPROTO_IP, unix.IP_TTL, ttl)
	})

	err = rawConn.Control(func(rawFd uintptr) {
		ttlErr = unix.SetsockoptInt(int(rawFd), unix.IPPROTO_IP, unix.IP_TTL, int(config.Ttl))
	})

	if err != nil {
		return errors.New("failed to get raw connection control").Base(err)
	}
	if ttlErr != nil {
		return errors.New("failed to set IP_TTL").Base(ttlErr)
	}

	var p [2]int
	pipeErr := unix.Pipe(p[:])
	if pipeErr != nil {
		return errors.New("failed to create pipe").Base(pipeErr)
	}
	defer unix.Close(p[0])
	defer unix.Close(p[1])

	cut := len(config.Payload) / 2
	if cut == 0 {
		cut = len(config.Payload)
	}
	iovs := []unix.Iovec{
		{Base: &config.Payload[0], Len: uint64(cut)},
		{Base: &config.Payload[cut], Len: uint64(len(config.Payload) - cut)},
	}

	if config.SendPayload {
		for _, iov := range iovs {
			if iov.Len == 0 {
				continue
			}
			_, pipeErr = unix.Vmsplice(p[1], []unix.Iovec{iov}, 0)
			if pipeErr != nil {
				return errors.New("failed to vmsplice").Base(pipeErr)
			}
			_, pipeErr = unix.Splice(p[0], nil, fd, nil, int(iov.Len), unix.SPLICE_F_GIFT)
			if pipeErr != nil {
				return errors.New("failed to splice").Base(pipeErr)
			}
		}
	}

	time.Sleep(time.Duration(config.Delay) * time.Millisecond)

	return nil
}
