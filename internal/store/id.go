package store
import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)
var counter uint64
func NewID(prefix string) string {
	ts := uint64(time.Now().UnixNano())
	c := atomic.AddUint64(&counter, 1)
	var b [4]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%s_%s%s", prefix, hex.EncodeToString(b[:]),
		fmt.Sprintf("%013x%04x", ts, c%0x10000))
}
func newID(prefix string) string { return NewID(prefix) }
