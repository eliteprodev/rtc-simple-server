module github.com/aler9/rtsp-simple-server

go 1.16

require (
	github.com/alecthomas/template v0.0.0-20190718012654-fb15b899a751 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d // indirect
	github.com/aler9/gortsplib v0.0.0-20210626112538-649c63cf5b62
	github.com/asticode/go-astits v1.9.0
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fsnotify/fsnotify v1.4.9
	github.com/gookit/color v1.4.2
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51
	github.com/notedit/rtmp v0.0.2
	github.com/pion/rtp v1.6.2
	github.com/stretchr/testify v1.6.1
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v2 v2.2.8
)

replace github.com/notedit/rtmp => github.com/aler9/rtmp v0.0.0-20210403095203-3be4a5535927
