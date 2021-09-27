package conf

import (
	"encoding/json"
	"fmt"
)

// Protocol is a RTSP stream protocol.
type Protocol int

// supported RTSP protocols.
const (
	ProtocolUDP Protocol = iota
	ProtocolMulticast
	ProtocolTCP
)

// Protocols is the protocols parameter.
type Protocols map[Protocol]struct{}

// MarshalJSON marshals a Protocols into JSON.
func (d Protocols) MarshalJSON() ([]byte, error) {
	out := make([]string, len(d))

	for p := range d {
		var v string

		switch p {
		case ProtocolUDP:
			v = "udp"

		case ProtocolMulticast:
			v = "multicast"

		default:
			v = "tcp"
		}

		out = append(out, v)
	}

	return json.Marshal(out)
}

// UnmarshalJSON unmarshals a Protocols from JSON.
func (d *Protocols) UnmarshalJSON(b []byte) error {
	var in []string
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}

	*d = make(Protocols)

	for _, proto := range in {
		switch proto {
		case "udp":
			(*d)[ProtocolUDP] = struct{}{}

		case "multicast":
			(*d)[ProtocolMulticast] = struct{}{}

		case "tcp":
			(*d)[ProtocolTCP] = struct{}{}

		default:
			return fmt.Errorf("invalid protocol: %s", proto)
		}
	}

	return nil
}
