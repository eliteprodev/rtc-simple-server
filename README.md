
<h1 align="center">
    <img src="logo.png" alt="rtsp-simple-server">
</h1>

[![Test](https://github.com/aler9/rtsp-simple-server/workflows/test/badge.svg)](https://github.com/aler9/rtsp-simple-server/actions?query=workflow:test)
[![Lint](https://github.com/aler9/rtsp-simple-server/workflows/lint/badge.svg)](https://github.com/aler9/rtsp-simple-server/actions?query=workflow:lint)
[![CodeCov](https://codecov.io/gh/aler9/rtsp-simple-server/branch/main/graph/badge.svg)](https://codecov.io/gh/aler9/rtsp-simple-server/branch/main)
[![Release](https://img.shields.io/github/v/release/aler9/rtsp-simple-server)](https://github.com/aler9/rtsp-simple-server/releases)
[![Docker Hub](https://img.shields.io/badge/docker-aler9/rtsp--simple--server-blue)](https://hub.docker.com/r/aler9/rtsp-simple-server)

_rtsp-simple-server_ is a simple, ready-to-use and zero-dependency RTSP / RTMP / HLS server and proxy, a software that allows users to publish, read and proxy live video and audio streams. RTSP, RTMP and HLS are independent protocols that allows to perform these operations with the help of a server, that is contacted by both publishers and readers and relays the publisher's streams to the readers; in particular:

* RTSP is the fastest way to publish and receive streams
* RTMP allows to interact with legacy servers or software (like OBS Studio)
* HLS allows to embed streams into a web page

Features:

* Publish live streams with RTSP (UDP, TCP or TLS mode) or RTMP
* Read live streams with RTSP (UDP, UDP-multicast, TCP or TLS mode), RTMP or HLS
* Pull and serve streams from other RTSP or RTMP servers or cameras, always or on-demand (RTSP proxy)
* Streams are automatically converted from a protocol to another (for instance, it's possible to publish with RTSP and read with HLS)
* Each stream can have multiple video and audio tracks, encoded with any codec (including H264, H265, VP8, VP9, MPEG2, MP3, AAC, Opus, PCM, JPEG)
* Serve multiple streams at once in separate paths
* Authenticate readers and publishers
* Redirect readers to other RTSP servers (load balancing)
* Run custom commands when clients connect, disconnect, read or publish streams
* Reload the configuration without disconnecting existing clients (hot reloading)
* Compatible with Linux, Windows and macOS, does not require any dependency or interpreter, it's a single executable

## Table of contents

* [Installation](#installation)
  * [Standard](#standard)
  * [Docker](#docker)
* [Basic usage](#basic-usage)
* [Advanced usage and FAQs](#advanced-usage-and-faqs)
  * [Configuration](#configuration)
  * [Encryption](#encryption)
  * [Authentication](#authentication)
  * [Encrypt the configuration](#encrypt-the-configuration)
  * [Proxy mode](#proxy-mode)
  * [RTMP protocol](#rtmp-protocol)
  * [HLS protocol](#hls-protocol)
  * [Publish from OBS Studio](#publish-from-obs-studio)
  * [Publish a webcam](#publish-a-webcam)
  * [Publish a Raspberry Pi Camera](#publish-a-raspberry-pi-camera)
  * [Remuxing, re-encoding, compression](#remuxing-re-encoding-compression)
  * [On-demand publishing](#on-demand-publishing)
  * [Redirect to another server](#redirect-to-another-server)
  * [Fallback stream](#fallback-stream)
  * [Start on boot with systemd](#start-on-boot-with-systemd)
  * [Monitoring](#monitoring)
  * [Corrupted frames](#corrupted-frames)
  * [Command-line usage](#command-line-usage)
  * [Compile and run from source](#compile-and-run-from-source)
* [Links](#links)

## Installation

### Standard

1. Download and extract a precompiled binary from the [release page](https://github.com/aler9/rtsp-simple-server/releases).

2. Start the server:

   ```
   ./rtsp-simple-server
   ```

### Docker

Download and launch the image:

```
docker run --rm -it --network=host aler9/rtsp-simple-server
```

The `--network=host` flag is mandatory since Docker can change the source port of UDP packets for routing reasons, and this doesn't allow to find out the publisher of the packets. This issue can be avoided by disabling UDP and exposing the RTSP port:

```
docker run --rm -it -e RTSP_PROTOCOLS=tcp -p 8554:8554 -p 1935:1935 aler9/rtsp-simple-server
```

## Basic usage

1. Publish a stream. For instance, you can publish a video/audio file with _FFmpeg_:

   ```
   ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:8554/mystream
   ```

   or _GStreamer_:

   ```
   gst-launch-1.0 rtspclientsink name=s location=rtsp://localhost:8554/mystream filesrc location=file.mp4 ! qtdemux name=d d.video_0 ! queue ! s.sink_0 d.audio_0 ! queue ! s.sink_1
   ```

2. Open the stream. For instance, you can open the stream with _VLC_:

   ```
   vlc rtsp://localhost:8554/mystream
   ```

   or _GStreamer_:

   ```
   gst-launch-1.0 rtspsrc location=rtsp://localhost:8554/mystream name=s s. ! application/x-rtp,media=video ! decodebin ! autovideosink s. ! application/x-rtp,media=audio ! decodebin ! audioconvert ! audioresample ! autoaudiosink
   ```

   or _FFmpeg_:

   ```
   ffmpeg -i rtsp://localhost:8554/mystream -c copy output.mp4
   ```

## Advanced usage and FAQs

### Configuration

All the configuration parameters are listed and commented in the [configuration file](rtsp-simple-server.yml).

There are two ways to change the configuration:

* By editing the `rtsp-simple-server.yml` file, that is

  * included into the release bundle
  * available in the root folder of the Docker image (`/rtsp-simple-server.yml`); it can be overridden in this way:

    ```
    docker run --rm -it --network=host -v $PWD/rtsp-simple-server.yml:/rtsp-simple-server.yml aler9/rtsp-simple-server
    ```

* By overriding configuration parameters with environment variables, in the format `RTSP_PARAMNAME`, where `PARAMNAME` is the uppercase name of a parameter. For instance, the `rtspAddress` parameter can be overridden in the following way:

   ```
   RTSP_RTSPADDRESS="127.0.0.1:8554" ./rtsp-simple-server
   ```

   Parameters in maps can be overridden by using underscores, in the following way:

   ```
   RTSP_PATHS_TEST_SOURCE=rtsp://myurl ./rtsp-simple-server
   ```

   This method is particularly useful when using Docker; any configuration parameter can be changed by passing environment variables with the `-e` flag:

   ```
   docker run --rm -it --network=host -e RTSP_PATHS_TEST_SOURCE=rtsp://myurl aler9/rtsp-simple-server
   ```

The configuration can be changed dinamically when the server is running (hot reloading) by writing to the configuration file. Changes are detected and applied without disconnecting existing clients, whenever it's possible.

### Encryption

Incoming and outgoing streams can be encrypted with TLS (obtaining the RTSPS protocol). A self-signed TLS certificate is needed and can be generated with openSSL:

```
openssl genrsa -out server.key 2048
openssl req -new -x509 -sha256 -key server.key -out server.crt -days 3650
```

Edit `rtsp-simple-server.yml`, and set the `protocols`, `encrypt`, `serverKey` and `serverCert` parameters:

```yml
protocols: [tcp]
encryption: optional
serverKey: server.key
serverCert: server.crt
```

Streams can then be published and read with the `rtsps` scheme and the `8555` port:

```
ffmpeg -i rtsps://ip:8555/...
```

If the client is _GStreamer_, disable the certificate validation:

```
gst-launch-1.0 rtspsrc location=rtsps://ip:8555/... tls-validation-flags=0
```

If the client is _VLC_, encryption can't be deployed, since _VLC_ doesn't support it.

### Authentication

Edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  all:
    publishUser: myuser
    publishPass: mypass
```

Only publishers that provide both username and password will be able to proceed:

```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://myuser:mypass@localhost:8554/mystream
```

It's possible to setup authentication for readers too:

```yml
paths:
  all:
    publishUser: myuser
    publishPass: mypass

    readUser: user
    readPass: userpass
```

If storing plain credentials in the configuration file is a security problem, username and passwords can be stored as sha256-hashed strings; a string must be hashed with sha256 and encoded with base64:

```
echo -n "userpass" | openssl dgst -binary -sha256 | openssl base64
```

Then stored with the `sha256:` prefix:

```yml
paths:
  all:
    readUser: sha256:j1tsRqDEw9xvq/D7/9tMx6Jh/jMhk3UfjwIB2f1zgMo=
    readPass: sha256:BdSWkrdV+ZxFBLUQQY7+7uv9RmiSVA8nrPmjGjJtZQQ=
```

**WARNING**: enable encryption or use a VPN to ensure that no one is intercepting the credentials.

### Encrypt the configuration

The configuration file can be entirely encrypted for security purposes.

An online encryption tool is [available here](https://play.golang.org/p/rX29jwObNe4).

The encryption procedure is the following:

1. NaCL's `crypto_secretbox` function is applied to the content of the configuration. NaCL is a cryptographic library available for [C/C++](https://nacl.cr.yp.to/secretbox.html), [Go](https://pkg.go.dev/golang.org/x/crypto/nacl/secretbox), [C#](https://github.com/somdoron/NaCl.net) and many other languages;

2. The string is prefixed with the nonce;

3. The string is encoded with base64.

After performing the encryption, it's enough to put the base64-encoded result into the configuration file, and launch the server with the `RTSP_CONFKEY` variable:

```
RTSP_CONFKEY=mykey ./rtsp-simple-server
```

### Proxy mode

_rtsp-simple-server_ is also a RTSP and RTMP proxy, that is usually deployed in one of these scenarios:

* when there are multiple users that are receiving a stream and the bandwidth is limited; the proxy is used to receive the stream once. Users can then connect to the proxy instead of the original source.
* when there's a NAT / firewall between a stream and the users; the proxy is installed on the NAT and makes the stream available to the outside world.

Edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  proxied:
    # url of the source stream, in the format rtsp://user:pass@host:port/path
    source: rtsp://original-url
```

After starting the server, users can connect to `rtsp://localhost:8554/proxied`, instead of connecting to the original url. The server supports any number of source streams, it's enough to add additional entries to the `paths` section:

```yml
paths:
  proxied1:
    source: rtsp://url1

  proxied2:
    source: rtsp://url1
```

It's possible to save bandwidth by enabling the on-demand mode: the stream will be pulled only when at least a client is connected:

```yml
paths:
  proxied:
    source: rtsp://original-url
    sourceOnDemand: yes
```

### RTMP protocol

RTMP is a protocol that is used to read and publish streams, but is less versatile and less efficient than RTSP (doesn't support UDP, encryption, doesn't support most RTSP codecs, doesn't support feedback mechanism). It is used when there's need of publishing or reading streams from a software that supports only RTMP (for instance, OBS Studio and DJI drones).

At the moment, only the H264 and AAC codecs can be used with the RTMP protocol.

Streams can be published or read with the RTMP protocol, for instance with _FFmpeg_:

```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f flv rtmp://localhost/mystream
```

or _GStreamer_:

```
gst-launch-1.0 -v flvmux name=s ! rtmpsink location=rtmp://localhost/mystream filesrc location=file.mp4 ! qtdemux name=d d.video_0 ! queue ! s.video d.audio_0 ! queue ! s.audio
```

Credentials can be provided by appending to the URL the `user` and `pass` parameters:

```
ffmpeg -re -stream_loop -1 -i file.ts -c copy -f flv rtmp://localhost:8554/mystream?user=myuser&pass=mypass
```

### HLS protocol

HLS is a media format that allows to embed live streams into web pages, inside standard `<video>` HTML tags. Every stream published to the server can be accessed with a web browser by visiting

```
http://localhost:8888/mystream
```

where `mystream` is the name of a stream that is being published.

### Publish from OBS Studio

In `Settings -> Stream` (or in the Auto-configuration Wizard), use the following parameters:

* Service: `Custom...`
* Server: `rtmp://localhost`
* Stream key: `mystream`

If credentials are in use, use the following parameters:

* Service: `Custom...`
* Server: `rtmp://localhost`
* Stream key: `mystream?user=myuser&pass=mypass`

### Publish a webcam

Edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  cam:
    runOnInit: ffmpeg -f v4l2 -i /dev/video0 -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH
    runOnInitRestart: yes
```

If the platform is Windows:

```yml
paths:
  cam:
    runOnInit: ffmpeg -f dshow -i video="USB2.0 HD UVC WebCam" -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH
    runOnInitRestart: yes
```

Where `USB2.0 HD UVC WebCam` is the name of your webcam, that can be obtained with:
```
ffmpeg -list_devices true -f dshow -i dummy
```

After starting the server, the webcam can be reached on `rtsp://localhost:8554/cam`.

### Publish a Raspberry Pi Camera

Install dependencies:

1. Gstreamer

   ```
   sudo apt install -y gstreamer1.0-tools gstreamer1.0-rtsp
   ```

2. gst-rpicamsrc, by following [instruction here](https://github.com/thaytan/gst-rpicamsrc)

Then edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  cam:
    runOnInit: gst-launch-1.0 rpicamsrc preview=false bitrate=2000000 keyframe-interval=50 ! video/x-h264,width=1920,height=1080,framerate=25/1 ! h264parse ! rtspclientsink location=rtsp://localhost:$RTSP_PORT/$RTSP_PATH
    runOnInitRestart: yes
```

After starting the server, the camera is available on `rtsp://localhost:8554/cam`.

### Remuxing, re-encoding, compression

To change the format, codec or compression of a stream, use _FFmpeg_ or _Gstreamer_ together with _rtsp-simple-server_. For instance, to re-encode an existing stream, that is available in the `/original` path, and publish the resulting stream in the `/compressed` path, edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  all:
  original:
    runOnPublish: ffmpeg -i rtsp://localhost:$RTSP_PORT/$RTSP_PATH -c:v libx264 -preset ultrafast -b:v 500k -max_muxing_queue_size 1024 -f rtsp rtsp://localhost:$RTSP_PORT/compressed
    runOnPublishRestart: yes
```

### On-demand publishing

Edit `rtsp-simple-server.yml` and replace everything inside section `paths` with the following content:

```yml
paths:
  ondemand:
    runOnDemand: ffmpeg -re -stream_loop -1 -i file.ts -c copy -f rtsp rtsp://localhost:$RTSP_PORT/$RTSP_PATH
    runOnDemandRestart: yes
```

The command inserted into `runOnDemand` will start only when a client requests the path `ondemand`, therefore the file will start streaming only when requested.

### Redirect to another server

To redirect to another server, use the `redirect` source:

```yml
paths:
  redirected:
    source: redirect
    sourceRedirect: rtsp://otherurl/otherpath
```

### Fallback stream

If no one is publishing to the server, readers can be redirected to a fallback path or URL that is serving a fallback stream:

```yml
paths:
  withfallback:
    fallback: /otherpath
```

### Start on boot with systemd

Systemd is the service manager used by Ubuntu, Debian and many other Linux distributions, and allows to launch rtsp-simple-server on boot.

Download a release bundle from the [release page](https://github.com/aler9/rtsp-simple-server/releases), unzip it, and move the executable and configuration in the system:

```
sudo mv rtsp-simple-server /usr/local/bin/
sudo mv rtsp-simple-server.yml /usr/local/etc/
```

Create the service:

```
sudo tee /etc/systemd/system/rtsp-simple-server.service >/dev/null << EOF
[Unit]
After=network.target
[Service]
ExecStart=/usr/local/bin/rtsp-simple-server /usr/local/etc/rtsp-simple-server.yml
[Install]
WantedBy=multi-user.target
EOF
```

Enable and start the service:

```
sudo systemctl enable rtsp-simple-server
sudo systemctl start rtsp-simple-server
```

### Monitoring

There are multiple ways to monitor the server usage over time:

* The current number of clients, publishers and readers is printed in each log line; for instance, the line:

  ```
  2020/01/01 00:00:00 [3/2] [conn 127.0.0.1:44428] OPTION
  ```

  means that there are 3 publishers and 2 readers.

* A metrics exporter, compatible with Prometheus, can be enabled with the parameter `metrics: yes`; then the server can be queried for metrics with Prometheus or with a simple HTTP request:

  ```
  wget -qO- localhost:9998/metrics
  ```

  Obtaining:

  ```
  rtsp_clients{state="publishing"} 15 1596122687740
  rtsp_clients{state="reading"} 8 1596122687740
  rtsp_sources{type="rtsp",state="idle"} 3 1596122687740
  rtsp_sources{type="rtsp",state="running"} 2 1596122687740
  rtsp_sources{type="rtmp",state="idle"} 1 1596122687740
  rtsp_sources{type="rtmp",state="running"} 0 1596122687740
  ```

  where:

  * `rtsp_clients{state="publishing"}` is the count of clients that are publishing
  * `rtsp_clients{state="reading"}` is the count of clients that are reading
  * `rtsp_sources{type="rtsp",state="idle"}` is the count of rtsp sources that are not running
  * `rtsp_sources{type="rtsp",state="running"}` is the count of rtsp sources that are running
  * `rtsp_sources{type="rtmp",state="idle"}` is the count of rtmp sources that are not running
  * `rtsp_sources{type="rtmp",state="running"}` is the count of rtmp sources that are running

* A performance monitor, compatible with pprof, can be enabled with the parameter `pprof: yes`; then the server can be queried for metrics with pprof-compatible tools, like:

  ```
  go tool pprof -text http://localhost:9999/debug/pprof/goroutine
  go tool pprof -text http://localhost:9999/debug/pprof/heap
  go tool pprof -text http://localhost:9999/debug/pprof/profile?seconds=30
  ```

### Corrupted frames

In some scenarios, the server can send incomplete or corrupted frames. This can be caused by multiple reasons:

* the packet buffer of the server is too small and can't handle the stream throughput. A solution consists in increasing its size:

  ```yml
  readBufferCount: 1024
  ```

* The stream throughput is too big and the stream can't be sent correctly with the UDP stream protocol. UDP is more performant, faster and more efficient than TCP, but doesn't have a retransmission mechanism, that is needed in case of streams that need a large bandwidth. A solution consists in switching to TCP:

  ```yml
  protocols: [tcp]
  ```

  In case the source is a camera:

  ```yml
  paths:
    test:
      source: rtsp://..
      sourceProtocol: tcp
  ```

* the software that is generating the stream (a camera or FFmpeg) is generating non-conformant RTP packets, with a payload bigger than the maximum allowed (that is 1460 due to the UDP MTU). A solution consists in increasing the buffer size:

  ```yml
  readBufferSize: 8192
  ```

### Command-line usage

```
usage: rtsp-simple-server [<flags>]

rtsp-simple-server v0.0.0

RTSP server.

Flags:
  --help     Show context-sensitive help (also try --help-long and --help-man).
  --version  print version

Args:
  [<confpath>]  path to a config file. The default is rtsp-simple-server.yml.
```

### Compile and run from source

Install Go 1.16, download the repository, open a terminal in it and run:

```
go run .
```

You can perform the entire operation inside Docker:

```
make run
```

## Links

Related projects

* https://github.com/aler9/gortsplib (RTSP library used internally)
* https://github.com/pion/sdp (SDP library used internally)
* https://github.com/pion/rtcp (RTCP library used internally)
* https://github.com/pion/rtp (RTP library used internally)
* https://github.com/notedit/rtmp (RTMP library used internally)
* https://github.com/flaviostutz/rtsp-relay

IETF Standards

* RTSP 1.0 https://tools.ietf.org/html/rfc2326
* RTSP 2.0 https://tools.ietf.org/html/rfc7826
* HTTP 1.1 https://tools.ietf.org/html/rfc2616

Conventions

* https://github.com/golang-standards/project-layout
