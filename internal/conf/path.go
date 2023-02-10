package conf

import (
	"encoding/json"
	"fmt"
	gourl "net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aler9/gortsplib/v2/pkg/url"
)

var rePathName = regexp.MustCompile(`^[0-9a-zA-Z_\-/\.~]+$`)

// IsValidPathName checks if a path name is valid.
func IsValidPathName(name string) error {
	if name == "" {
		return fmt.Errorf("cannot be empty")
	}

	if name[0] == '/' {
		return fmt.Errorf("can't begin with a slash")
	}

	if name[len(name)-1] == '/' {
		return fmt.Errorf("can't end with a slash")
	}

	if !rePathName.MatchString(name) {
		return fmt.Errorf("can contain only alphanumeric characters, underscore, dot, tilde, minus or slash")
	}

	return nil
}

// PathConf is a path configuration.
type PathConf struct {
	Regexp *regexp.Regexp `json:"-"`

	// source
	Source                     string         `json:"source"`
	SourceProtocol             SourceProtocol `json:"sourceProtocol"`
	SourceAnyPortEnable        bool           `json:"sourceAnyPortEnable"`
	SourceFingerprint          string         `json:"sourceFingerprint"`
	SourceOnDemand             bool           `json:"sourceOnDemand"`
	SourceOnDemandStartTimeout StringDuration `json:"sourceOnDemandStartTimeout"`
	SourceOnDemandCloseAfter   StringDuration `json:"sourceOnDemandCloseAfter"`
	SourceRedirect             string         `json:"sourceRedirect"`
	DisablePublisherOverride   bool           `json:"disablePublisherOverride"`
	Fallback                   string         `json:"fallback"`
	RPICameraCamID             int            `json:"rpiCameraCamID"`
	RPICameraWidth             int            `json:"rpiCameraWidth"`
	RPICameraHeight            int            `json:"rpiCameraHeight"`
	RPICameraHFlip             bool           `json:"rpiCameraHFlip"`
	RPICameraVFlip             bool           `json:"rpiCameraVFlip"`
	RPICameraBrightness        float64        `json:"rpiCameraBrightness"`
	RPICameraContrast          float64        `json:"rpiCameraContrast"`
	RPICameraSaturation        float64        `json:"rpiCameraSaturation"`
	RPICameraSharpness         float64        `json:"rpiCameraSharpness"`
	RPICameraExposure          string         `json:"rpiCameraExposure"`
	RPICameraAWB               string         `json:"rpiCameraAWB"`
	RPICameraDenoise           string         `json:"rpiCameraDenoise"`
	RPICameraShutter           int            `json:"rpiCameraShutter"`
	RPICameraMetering          string         `json:"rpiCameraMetering"`
	RPICameraGain              float64        `json:"rpiCameraGain"`
	RPICameraEV                float64        `json:"rpiCameraEV"`
	RPICameraROI               string         `json:"rpiCameraROI"`
	RPICameraTuningFile        string         `json:"rpiCameraTuningFile"`
	RPICameraMode              string         `json:"rpiCameraMode"`
	RPICameraFPS               int            `json:"rpiCameraFPS"`
	RPICameraIDRPeriod         int            `json:"rpiCameraIDRPeriod"`
	RPICameraBitrate           int            `json:"rpiCameraBitrate"`
	RPICameraProfile           string         `json:"rpiCameraProfile"`
	RPICameraLevel             string         `json:"rpiCameraLevel"`
	RPICameraAfMode            string         `json:"rpiCameraAfMode"`
	RPICameraAfRange           string         `json:"rpiCameraAfRange"`
	RPICameraAfSpeed           string         `json:"rpiCameraAfSpeed"`
	RPICameraLensPosition      float64        `json:"rpiCameraLensPosition"`
	RPICameraAfWindow          string         `json:"rpiCameraAfWindow"`

	// authentication
	PublishUser Credential `json:"publishUser"`
	PublishPass Credential `json:"publishPass"`
	PublishIPs  IPsOrCIDRs `json:"publishIPs"`
	ReadUser    Credential `json:"readUser"`
	ReadPass    Credential `json:"readPass"`
	ReadIPs     IPsOrCIDRs `json:"readIPs"`

	// external commands
	RunOnInit               string         `json:"runOnInit"`
	RunOnInitRestart        bool           `json:"runOnInitRestart"`
	RunOnDemand             string         `json:"runOnDemand"`
	RunOnDemandRestart      bool           `json:"runOnDemandRestart"`
	RunOnDemandStartTimeout StringDuration `json:"runOnDemandStartTimeout"`
	RunOnDemandCloseAfter   StringDuration `json:"runOnDemandCloseAfter"`
	RunOnReady              string         `json:"runOnReady"`
	RunOnReadyRestart       bool           `json:"runOnReadyRestart"`
	RunOnRead               string         `json:"runOnRead"`
	RunOnReadRestart        bool           `json:"runOnReadRestart"`
}

func (pconf *PathConf) checkAndFillMissing(conf *Conf, name string) error {
	// normal path
	if name == "" || name[0] != '~' {
		err := IsValidPathName(name)
		if err != nil {
			return fmt.Errorf("invalid path name '%s': %s", name, err)
		}

		// regular expression path
	} else {
		pathRegexp, err := regexp.Compile(name[1:])
		if err != nil {
			return fmt.Errorf("invalid regular expression: %s", name[1:])
		}
		pconf.Regexp = pathRegexp
	}

	if pconf.Source == "" {
		pconf.Source = "publisher"
	}

	switch {
	case pconf.Source == "publisher":

	case strings.HasPrefix(pconf.Source, "rtsp://") ||
		strings.HasPrefix(pconf.Source, "rtsps://"):
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTSP source. use another path")
		}

		_, err := url.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid RTSP URL", pconf.Source)
		}

	case strings.HasPrefix(pconf.Source, "rtmp://") ||
		strings.HasPrefix(pconf.Source, "rtmps://"):
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a RTMP source. use another path")
		}

		u, err := gourl.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid RTMP URL", pconf.Source)
		}

		if u.User != nil {
			pass, _ := u.User.Password()
			user := u.User.Username()
			if user != "" && pass == "" ||
				user == "" && pass != "" {
				return fmt.Errorf("username and password must be both provided")
			}
		}

	case strings.HasPrefix(pconf.Source, "http://") ||
		strings.HasPrefix(pconf.Source, "https://"):
		if pconf.Regexp != nil {
			return fmt.Errorf("a path with a regular expression (or path 'all') cannot have a HLS source. use another path")
		}

		u, err := gourl.Parse(pconf.Source)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid HLS URL", pconf.Source)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("'%s' is not a valid HLS URL", pconf.Source)
		}

		if u.User != nil {
			pass, _ := u.User.Password()
			user := u.User.Username()
			if user != "" && pass == "" ||
				user == "" && pass != "" {
				return fmt.Errorf("username and password must be both provided")
			}
		}

	case pconf.Source == "redirect":
		if pconf.SourceRedirect == "" {
			return fmt.Errorf("source redirect must be filled")
		}

		_, err := url.Parse(pconf.SourceRedirect)
		if err != nil {
			return fmt.Errorf("'%s' is not a valid RTSP URL", pconf.SourceRedirect)
		}

	case pconf.Source == "rpiCamera":
		if pconf.Regexp != nil {
			return fmt.Errorf(
				"a path with a regular expression (or path 'all') cannot have 'rpiCamera' as source. use another path")
		}

		for otherName, otherPath := range conf.Paths {
			if otherPath != pconf && otherPath != nil && otherPath.Source == "rpiCamera" {
				return fmt.Errorf("'rpiCamera' is used as source in two paths ('%s' and '%s')", name, otherName)
			}
		}

		if pconf.RPICameraWidth == 0 {
			pconf.RPICameraWidth = 1920
		}
		if pconf.RPICameraHeight == 0 {
			pconf.RPICameraHeight = 1080
		}
		if pconf.RPICameraContrast == 0 {
			pconf.RPICameraContrast = 1
		}
		if pconf.RPICameraSaturation == 0 {
			pconf.RPICameraSaturation = 1
		}
		if pconf.RPICameraSharpness == 0 {
			pconf.RPICameraSharpness = 1
		}
		if pconf.RPICameraFPS == 0 {
			pconf.RPICameraFPS = 30
		}
		if pconf.RPICameraIDRPeriod == 0 {
			pconf.RPICameraIDRPeriod = 60
		}
		if pconf.RPICameraBitrate == 0 {
			pconf.RPICameraBitrate = 1000000
		}
		if pconf.RPICameraProfile == "" {
			pconf.RPICameraProfile = "main"
		}
		if pconf.RPICameraLevel == "" {
			pconf.RPICameraLevel = "4.1"
		}

	default:
		return fmt.Errorf("invalid source: '%s'", pconf.Source)
	}

	if pconf.SourceOnDemand {
		if pconf.Source == "publisher" {
			return fmt.Errorf("'sourceOnDemand' is useless when source is 'publisher'")
		}
	}

	if pconf.SourceOnDemandStartTimeout == 0 {
		pconf.SourceOnDemandStartTimeout = 10 * StringDuration(time.Second)
	}

	if pconf.SourceOnDemandCloseAfter == 0 {
		pconf.SourceOnDemandCloseAfter = 10 * StringDuration(time.Second)
	}

	if pconf.Fallback != "" {
		if strings.HasPrefix(pconf.Fallback, "/") {
			err := IsValidPathName(pconf.Fallback[1:])
			if err != nil {
				return fmt.Errorf("'%s': %s", pconf.Fallback, err)
			}
		} else {
			_, err := url.Parse(pconf.Fallback)
			if err != nil {
				return fmt.Errorf("'%s' is not a valid RTSP URL", pconf.Fallback)
			}
		}
	}

	if (pconf.PublishUser != "" && pconf.PublishPass == "") ||
		(pconf.PublishUser == "" && pconf.PublishPass != "") {
		return fmt.Errorf("read username and password must be both filled")
	}

	if pconf.PublishUser != "" && pconf.Source != "publisher" {
		return fmt.Errorf("'publishUser' is useless when source is not 'publisher', since " +
			"the stream is not provided by a publisher, but by a fixed source")
	}

	if pconf.PublishUser != "" && conf.ExternalAuthenticationURL != "" {
		return fmt.Errorf("'publishUser' can't be used with 'externalAuthenticationURL'")
	}

	if len(pconf.PublishIPs) > 0 && pconf.Source != "publisher" {
		return fmt.Errorf("'publishIPs' is useless when source is not 'publisher', since " +
			"the stream is not provided by a publisher, but by a fixed source")
	}

	if len(pconf.PublishIPs) > 0 && conf.ExternalAuthenticationURL != "" {
		return fmt.Errorf("'publishIPs' can't be used with 'externalAuthenticationURL'")
	}

	if (pconf.ReadUser != "" && pconf.ReadPass == "") ||
		(pconf.ReadUser == "" && pconf.ReadPass != "") {
		return fmt.Errorf("read username and password must be both filled")
	}

	if pconf.ReadUser != "" && conf.ExternalAuthenticationURL != "" {
		return fmt.Errorf("'readUser' can't be used with 'externalAuthenticationURL'")
	}

	if len(pconf.ReadIPs) > 0 && conf.ExternalAuthenticationURL != "" {
		return fmt.Errorf("'readIPs' can't be used with 'externalAuthenticationURL'")
	}

	if pconf.RunOnInit != "" && pconf.Regexp != nil {
		return fmt.Errorf("a path with a regular expression does not support option 'runOnInit'; use another path")
	}

	if pconf.RunOnDemand != "" && pconf.Source != "publisher" {
		return fmt.Errorf("'runOnDemand' can be used only when source is 'publisher'")
	}

	if pconf.RunOnDemandStartTimeout == 0 {
		pconf.RunOnDemandStartTimeout = 10 * StringDuration(time.Second)
	}

	if pconf.RunOnDemandCloseAfter == 0 {
		pconf.RunOnDemandCloseAfter = 10 * StringDuration(time.Second)
	}

	return nil
}

// Equal checks whether two PathConfs are equal.
func (pconf *PathConf) Equal(other *PathConf) bool {
	a, _ := json.Marshal(pconf)
	b, _ := json.Marshal(other)
	return string(a) == string(b)
}
