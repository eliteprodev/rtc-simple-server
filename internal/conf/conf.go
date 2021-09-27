package conf

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/aler9/gortsplib/pkg/headers"
	"golang.org/x/crypto/nacl/secretbox"
	"gopkg.in/yaml.v2"

	"github.com/aler9/rtsp-simple-server/internal/logger"
)

func decrypt(key string, byts []byte) ([]byte, error) {
	enc, err := base64.StdEncoding.DecodeString(string(byts))
	if err != nil {
		return nil, err
	}

	var secretKey [32]byte
	copy(secretKey[:], key)

	var decryptNonce [24]byte
	copy(decryptNonce[:], enc[:24])
	decrypted, ok := secretbox.Open(nil, enc[24:], &decryptNonce, &secretKey)
	if !ok {
		return nil, fmt.Errorf("decryption error")
	}

	return decrypted, nil
}

func loadFromFile(fpath string, conf *Conf) (bool, error) {
	// rtsp-simple-server.yml is optional
	// other configuration files are not
	if fpath == "rtsp-simple-server.yml" {
		if _, err := os.Stat(fpath); err != nil {
			return false, nil
		}
	}

	byts, err := ioutil.ReadFile(fpath)
	if err != nil {
		return true, err
	}

	if key, ok := os.LookupEnv("RTSP_CONFKEY"); ok {
		byts, err = decrypt(key, byts)
		if err != nil {
			return true, err
		}
	}

	// load YAML config into a generic map
	var temp interface{}
	err = yaml.Unmarshal(byts, &temp)
	if err != nil {
		return true, err
	}

	// convert interface{} keys into string keys to avoid JSON errors
	var convert func(i interface{}) interface{}
	convert = func(i interface{}) interface{} {
		switch x := i.(type) {
		case map[interface{}]interface{}:
			m2 := map[string]interface{}{}
			for k, v := range x {
				m2[k.(string)] = convert(v)
			}
			return m2
		case []interface{}:
			a2 := make([]interface{}, len(x))
			for i, v := range x {
				a2[i] = convert(v)
			}
			return a2
		}
		return i
	}
	temp = convert(temp)

	// convert the generic map into JSON
	byts, err = json.Marshal(temp)
	if err != nil {
		return true, err
	}

	// load the configuration from JSON
	err = json.Unmarshal(byts, conf)
	if err != nil {
		return true, err
	}

	return true, nil
}

// Conf is a configuration.
type Conf struct {
	// general
	LogLevel            LogLevel        `json:"logLevel"`
	LogDestinations     LogDestinations `json:"logDestinations"`
	LogFile             string          `json:"logFile"`
	ReadTimeout         StringDuration  `json:"readTimeout"`
	WriteTimeout        StringDuration  `json:"writeTimeout"`
	ReadBufferCount     int             `json:"readBufferCount"`
	API                 bool            `json:"api"`
	APIAddress          string          `json:"apiAddress"`
	Metrics             bool            `json:"metrics"`
	MetricsAddress      string          `json:"metricsAddress"`
	PPROF               bool            `json:"pprof"`
	PPROFAddress        string          `json:"pprofAddress"`
	RunOnConnect        string          `json:"runOnConnect"`
	RunOnConnectRestart bool            `json:"runOnConnectRestart"`

	// RTSP
	RTSPDisable       bool        `json:"rtspDisable"`
	Protocols         Protocols   `json:"protocols"`
	Encryption        Encryption  `json:"encryption"`
	RTSPAddress       string      `json:"rtspAddress"`
	RTSPSAddress      string      `json:"rtspsAddress"`
	RTPAddress        string      `json:"rtpAddress"`
	RTCPAddress       string      `json:"rtcpAddress"`
	MulticastIPRange  string      `json:"multicastIPRange"`
	MulticastRTPPort  int         `json:"multicastRTPPort"`
	MulticastRTCPPort int         `json:"multicastRTCPPort"`
	ServerKey         string      `json:"serverKey"`
	ServerCert        string      `json:"serverCert"`
	AuthMethods       AuthMethods `json:"authMethods"`
	ReadBufferSize    int         `json:"readBufferSize"`

	// RTMP
	RTMPDisable bool   `json:"rtmpDisable"`
	RTMPAddress string `json:"rtmpAddress"`

	// HLS
	HLSDisable         bool           `json:"hlsDisable"`
	HLSAddress         string         `json:"hlsAddress"`
	HLSAlwaysRemux     bool           `json:"hlsAlwaysRemux"`
	HLSSegmentCount    int            `json:"hlsSegmentCount"`
	HLSSegmentDuration StringDuration `json:"hlsSegmentDuration"`
	HLSAllowOrigin     string         `json:"hlsAllowOrigin"`

	// paths
	Paths map[string]*PathConf `json:"paths"`
}

// Load loads a Conf.
func Load(fpath string) (*Conf, bool, error) {
	conf := &Conf{}

	found, err := loadFromFile(fpath, conf)
	if err != nil {
		return nil, false, err
	}

	err = loadFromEnvironment("RTSP", conf)
	if err != nil {
		return nil, false, err
	}

	err = conf.CheckAndFillMissing()
	if err != nil {
		return nil, false, err
	}

	return conf, found, nil
}

// CheckAndFillMissing checks the configuration for errors and fills missing fields.
func (conf *Conf) CheckAndFillMissing() error {
	if conf.LogLevel == 0 {
		conf.LogLevel = LogLevel(logger.Info)
	}

	if len(conf.LogDestinations) == 0 {
		conf.LogDestinations = LogDestinations{logger.DestinationStdout: {}}
	}

	if conf.LogFile == "" {
		conf.LogFile = "rtsp-simple-server.log"
	}

	if conf.ReadTimeout == 0 {
		conf.ReadTimeout = 10 * StringDuration(time.Second)
	}

	if conf.WriteTimeout == 0 {
		conf.WriteTimeout = 10 * StringDuration(time.Second)
	}

	if conf.ReadBufferCount == 0 {
		conf.ReadBufferCount = 512
	}

	if conf.APIAddress == "" {
		conf.APIAddress = "127.0.0.1:9997"
	}

	if conf.MetricsAddress == "" {
		conf.MetricsAddress = "127.0.0.1:9998"
	}

	if conf.PPROFAddress == "" {
		conf.PPROFAddress = "127.0.0.1:9999"
	}

	if len(conf.Protocols) == 0 {
		conf.Protocols = Protocols{
			ProtocolUDP:       {},
			ProtocolMulticast: {},
			ProtocolTCP:       {},
		}
	}

	if conf.Encryption == EncryptionStrict {
		if _, ok := conf.Protocols[ProtocolUDP]; ok {
			return fmt.Errorf("strict encryption can't be used with the UDP stream protocol")
		}
	}

	if conf.RTSPAddress == "" {
		conf.RTSPAddress = ":8554"
	}

	if conf.RTSPSAddress == "" {
		conf.RTSPSAddress = ":8555"
	}

	if conf.RTPAddress == "" {
		conf.RTPAddress = ":8000"
	}

	if conf.RTCPAddress == "" {
		conf.RTCPAddress = ":8001"
	}

	if conf.MulticastIPRange == "" {
		conf.MulticastIPRange = "224.1.0.0/16"
	}

	if conf.MulticastRTPPort == 0 {
		conf.MulticastRTPPort = 8002
	}

	if conf.MulticastRTCPPort == 0 {
		conf.MulticastRTCPPort = 8003
	}

	if conf.ServerKey == "" {
		conf.ServerKey = "server.key"
	}

	if conf.ServerCert == "" {
		conf.ServerCert = "server.crt"
	}

	if len(conf.AuthMethods) == 0 {
		conf.AuthMethods = AuthMethods{headers.AuthBasic, headers.AuthDigest}
	}

	if conf.RTMPAddress == "" {
		conf.RTMPAddress = ":1935"
	}

	if conf.HLSAddress == "" {
		conf.HLSAddress = ":8888"
	}

	if conf.HLSSegmentCount == 0 {
		conf.HLSSegmentCount = 3
	}

	if conf.HLSSegmentDuration == 0 {
		conf.HLSSegmentDuration = 1 * StringDuration(time.Second)
	}

	if conf.HLSAllowOrigin == "" {
		conf.HLSAllowOrigin = "*"
	}

	if len(conf.Paths) == 0 {
		conf.Paths = map[string]*PathConf{
			"all": {},
		}
	}

	// "all" is an alias for "~^.*$"
	if _, ok := conf.Paths["all"]; ok {
		conf.Paths["~^.*$"] = conf.Paths["all"]
		delete(conf.Paths, "all")
	}

	for name, pconf := range conf.Paths {
		if pconf == nil {
			conf.Paths[name] = &PathConf{}
			pconf = conf.Paths[name]
		}

		err := pconf.checkAndFillMissing(name)
		if err != nil {
			return err
		}
	}

	return nil
}
