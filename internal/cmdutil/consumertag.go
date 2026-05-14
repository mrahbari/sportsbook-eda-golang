package cmdutil

import (
	"crypto/rand"
	"encoding/hex"
)

// ConsumerTag returns a stable prefix plus random suffix for AMQP consumer tags.
func ConsumerTag(prefix string) string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return prefix + "-" + hex.EncodeToString(b[:])
}
