package rtmputils

import (
	"net"
	"net/url"

	"github.com/notedit/rtmp/av"
	"github.com/notedit/rtmp/format/rtmp"
)

// Conn contains a RTMP connection and a net connection.
type Conn struct {
	rconn *rtmp.Conn
	nconn net.Conn
}

// NewConn allocates a Conn.
func NewConn(rconn *rtmp.Conn, nconn net.Conn) *Conn {
	return &Conn{
		rconn: rconn,
		nconn: nconn,
	}
}

// NetConn returns the underlying net.Conn.
func (c *Conn) NetConn() net.Conn {
	return c.nconn
}

// IsPublishing returns whether the connection is publishing.
func (c *Conn) IsPublishing() bool {
	return c.rconn.Publishing
}

// URL returns the URL requested by the connection.
func (c *Conn) URL() *url.URL {
	return c.rconn.URL
}

// ReadPacket reads a packet.
func (c *Conn) ReadPacket() (av.Packet, error) {
	return c.rconn.ReadPacket()
}

// WritePacket writes a packet.
func (c *Conn) WritePacket(pkt av.Packet) error {
	err := c.rconn.WritePacket(pkt)
	if err != nil {
		return err
	}
	return c.rconn.FlushWrite()
}
