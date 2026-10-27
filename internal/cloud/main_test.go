package cloud

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestMain(testRun *testing.M) {
	keyring.MockInit()
	os.Exit(testRun.Run())
}
