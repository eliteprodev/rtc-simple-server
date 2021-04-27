package rtmp

import (
	"bufio"
	"context"
	"net"
	"net/url"

	"github.com/notedit/rtmp/format/rtmp"
)

// DialContext connects to a server in reading mode.
func DialContext(ctx context.Context, address string) (*Conn, error) {
	// https://github.com/aler9/rtmp/blob/master/format/rtmp/readpublisher.go#L74

	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	host := rtmp.UrlGetHost(u)

	var d net.Dialer
	nconn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}

	rw := &bufio.ReadWriter{
		Reader: bufio.NewReaderSize(nconn, 4096),
		Writer: bufio.NewWriterSize(nconn, 4096),
	}
	rconn := rtmp.NewConn(rw)
	rconn.URL = u

	return &Conn{
		rconn: rconn,
		nconn: nconn,
	}, nil
}

// ClientHandshake performs the handshake of a client-side connection.
func (c *Conn) ClientHandshake() error {
	return c.rconn.Prepare(rtmp.StageGotPublishOrPlayCommand, rtmp.PrepareReading)
}
