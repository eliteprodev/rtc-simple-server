package core

import (
	"crypto/tls"
	"os"
	"testing"
	"time"

	"github.com/aler9/gortsplib/v2"
	"github.com/aler9/gortsplib/v2/pkg/auth"
	"github.com/aler9/gortsplib/v2/pkg/base"
	"github.com/aler9/gortsplib/v2/pkg/media"
	"github.com/aler9/gortsplib/v2/pkg/url"
	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

type testServer struct {
	onDescribe func(*gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error)
	onSetup    func(*gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error)
	onPlay     func(*gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error)
}

func (sh *testServer) OnDescribe(ctx *gortsplib.ServerHandlerOnDescribeCtx,
) (*base.Response, *gortsplib.ServerStream, error) {
	return sh.onDescribe(ctx)
}

func (sh *testServer) OnSetup(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
	return sh.onSetup(ctx)
}

func (sh *testServer) OnPlay(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
	return sh.onPlay(ctx)
}

func TestRTSPSource(t *testing.T) {
	for _, source := range []string{
		"udp",
		"tcp",
		"tls",
	} {
		t.Run(source, func(t *testing.T) {
			medi := testMediaH264
			stream := gortsplib.NewServerStream(media.Medias{medi})

			var authValidator *auth.Validator

			s := gortsplib.Server{
				Handler: &testServer{
					onDescribe: func(ctx *gortsplib.ServerHandlerOnDescribeCtx,
					) (*base.Response, *gortsplib.ServerStream, error) {
						if authValidator == nil {
							authValidator = auth.NewValidator("testuser", "testpass", nil)
						}

						err := authValidator.ValidateRequest(ctx.Request, nil)
						if err != nil {
							return &base.Response{
								StatusCode: base.StatusUnauthorized,
								Header: base.Header{
									"WWW-Authenticate": authValidator.Header(),
								},
							}, nil, nil
						}

						return &base.Response{
							StatusCode: base.StatusOK,
						}, stream, nil
					},
					onSetup: func(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
						return &base.Response{
							StatusCode: base.StatusOK,
						}, stream, nil
					},
					onPlay: func(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
						go func() {
							time.Sleep(1 * time.Second)
							stream.WritePacketRTP(medi, &rtp.Packet{
								Header: rtp.Header{
									Version:        0x02,
									PayloadType:    96,
									SequenceNumber: 57899,
									Timestamp:      345234345,
									SSRC:           978651231,
									Marker:         true,
								},
								Payload: []byte{0x01, 0x02, 0x03, 0x04},
							})
						}()

						return &base.Response{
							StatusCode: base.StatusOK,
						}, nil
					},
				},
				RTSPAddress: "127.0.0.1:8555",
			}

			switch source {
			case "udp":
				s.UDPRTPAddress = "127.0.0.1:8002"
				s.UDPRTCPAddress = "127.0.0.1:8003"

			case "tls":
				serverCertFpath, err := writeTempFile(serverCert)
				require.NoError(t, err)
				defer os.Remove(serverCertFpath)

				serverKeyFpath, err := writeTempFile(serverKey)
				require.NoError(t, err)
				defer os.Remove(serverKeyFpath)

				cert, err := tls.LoadX509KeyPair(serverCertFpath, serverKeyFpath)
				require.NoError(t, err)

				s.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
			}

			err := s.Start()
			require.NoError(t, err)
			defer s.Wait()
			defer s.Close()

			if source == "udp" || source == "tcp" {
				p, ok := newInstance("paths:\n" +
					"  proxied:\n" +
					"    source: rtsp://testuser:testpass@localhost:8555/teststream\n" +
					"    sourceProtocol: " + source + "\n" +
					"    sourceOnDemand: yes\n")
				require.Equal(t, true, ok)
				defer p.Close()
			} else {
				p, ok := newInstance("paths:\n" +
					"  proxied:\n" +
					"    source: rtsps://testuser:testpass@localhost:8555/teststream\n" +
					"    sourceFingerprint: 33949E05FFFB5FF3E8AA16F8213A6251B4D9363804BA53233C4DA9A46D6F2739\n" +
					"    sourceOnDemand: yes\n")
				require.Equal(t, true, ok)
				defer p.Close()
			}

			received := make(chan struct{})

			c := gortsplib.Client{}

			u, err := url.Parse("rtsp://127.0.0.1:8554/proxied")
			require.NoError(t, err)

			err = c.Start(u.Scheme, u.Host)
			require.NoError(t, err)
			defer c.Close()

			medias, baseURL, _, err := c.Describe(u)
			require.NoError(t, err)

			err = c.SetupAll(medias, baseURL)
			require.NoError(t, err)

			c.OnPacketRTP(medias[0], medias[0].Formats[0], func(pkt *rtp.Packet) {
				require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, pkt.Payload)
				close(received)
			})

			_, err = c.Play(nil)
			require.NoError(t, err)

			<-received
		})
	}
}

func TestRTSPSourceNoPassword(t *testing.T) {
	medi := testMediaH264
	stream := gortsplib.NewServerStream(media.Medias{medi})

	var authValidator *auth.Validator
	done := make(chan struct{})

	s := gortsplib.Server{
		Handler: &testServer{
			onDescribe: func(ctx *gortsplib.ServerHandlerOnDescribeCtx) (*base.Response, *gortsplib.ServerStream, error) {
				if authValidator == nil {
					authValidator = auth.NewValidator("testuser", "", nil)
				}

				err := authValidator.ValidateRequest(ctx.Request, nil)
				if err != nil {
					return &base.Response{
						StatusCode: base.StatusUnauthorized,
						Header: base.Header{
							"WWW-Authenticate": authValidator.Header(),
						},
					}, nil, nil
				}

				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onSetup: func(ctx *gortsplib.ServerHandlerOnSetupCtx) (*base.Response, *gortsplib.ServerStream, error) {
				close(done)
				return &base.Response{
					StatusCode: base.StatusOK,
				}, stream, nil
			},
			onPlay: func(ctx *gortsplib.ServerHandlerOnPlayCtx) (*base.Response, error) {
				return &base.Response{
					StatusCode: base.StatusOK,
				}, nil
			},
		},
		RTSPAddress: "127.0.0.1:8555",
	}
	err := s.Start()
	require.NoError(t, err)
	defer s.Wait()
	defer s.Close()

	p, ok := newInstance("rtmpDisable: yes\n" +
		"hlsDisable: yes\n" +
		"webrtcDisable: yes\n" +
		"paths:\n" +
		"  proxied:\n" +
		"    source: rtsp://testuser:@127.0.0.1:8555/teststream\n" +
		"    sourceProtocol: tcp\n")
	require.Equal(t, true, ok)
	defer p.Close()

	<-done
}
