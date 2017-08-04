package transport

import (
	"github.com/gammazero/nexus/logger"
	"github.com/gammazero/nexus/wamp"
)

const defaultLinkedPeersOutQueueSize = 16

// LinkedPeers creates two connected peers.  Messages sent to one peer
// appear in the Recv of the other.
//
// This is used for connecting client sessions to the router.
//
// Exported since it is used in test code for creating in-process test clients.
func LinkedPeers(outQueueSize int, logger logger.Logger) (wamp.Peer, wamp.Peer) {
	if outQueueSize < 1 {
		outQueueSize = defaultLinkedPeersOutQueueSize
	}

	// The channel used for the router to send messages to the client should be
	// large enough to prevent blocking while waiting for a slow client, as a
	// client may block on I/O.  If the client does block, then the message
	// should be dropped.
	rToC := make(chan wamp.Message, outQueueSize)

	// Messages read from a client can usually be handled immediately, since
	// routing is fast and does not block on I/O.  Therefore this channle does
	// not need to be more than size 1.
	cToR := make(chan wamp.Message, 1)

	// router reads from and writes to client
	r := &localPeer{rd: cToR, wr: rToC, wrRtoC: true}
	// client reads from and writes to router
	c := &localPeer{rd: rToC, wr: cToR}

	return c, r
}

// localPeer implements Peer
type localPeer struct {
	wr     chan<- wamp.Message
	rd     <-chan wamp.Message
	wrRtoC bool
	log    logger.Logger
}

// Recv returns the channel this peer reads incoming messages from.
func (p *localPeer) Recv() <-chan wamp.Message { return p.rd }

// Send write a message to the channel the peer sends outgoing messages to.
func (p *localPeer) Send(msg wamp.Message) {
	if p.wrRtoC {
		select {
		case p.wr <- msg:
		default:
			p.log.Println("WARNING: client blocked router.  Dropped:",
				msg.MessageType())
		}
		return
	}
	// It is OK for the router to block a client since this will not block
	// other clients.
	p.wr <- msg
}

// Close closes the outgoing channel, waking any readers waiting on data from
// this peer.
func (p *localPeer) Close() { close(p.wr) }