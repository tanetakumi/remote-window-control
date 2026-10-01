package capture_test

import (
	"os"
	"testing"

	"share-app-host/test/testutil"
)

func TestMain(m *testing.M) {
	testutil.RunFakeProbeIfRequested()
	os.Exit(m.Run())
}
