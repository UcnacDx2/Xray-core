package scenarios

import (
	"flag"
	"testing"
)

var (
	keepProxyAlive = flag.Bool("keep-proxy-alive", false, "Keep the proxy alive after tests are finished.")
)

func TestMain(m *testing.M) {
	genTestBinaryPath()
	defer testBinaryCleanFn()

	m.Run()
}
