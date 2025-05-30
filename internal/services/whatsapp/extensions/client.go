// Package extensions provides custom extensions to the whatsmeow client
package extensions

import (
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// SetNewsletterPhoto sets the photo for a newsletter/channel
// This uses the same method as SetGroupPhoto since newsletters use the same underlying mechanism
func SetNewsletterPhoto(cli *whatsmeow.Client, jid types.JID, avatar []byte) (string, error) {
	// Use the same method as groups and communities since WhatsApp treats newsletters similarly
	// for photo updates at the protocol level
	return cli.SetGroupPhoto(jid, avatar)
}
