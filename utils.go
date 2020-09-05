package main

import (
	"fmt"
	"net"
	"regexp"
)

func parseIpCidrList(in []string) ([]interface{}, error) {
	if len(in) == 0 {
		return nil, nil
	}

	var ret []interface{}
	for _, t := range in {
		_, ipnet, err := net.ParseCIDR(t)
		if err == nil {
			ret = append(ret, ipnet)
			continue
		}

		ip := net.ParseIP(t)
		if ip != nil {
			ret = append(ret, ip)
			continue
		}

		return nil, fmt.Errorf("unable to parse ip/network '%s'", t)
	}
	return ret, nil
}

func ipEqualOrInRange(ip net.IP, ips []interface{}) bool {
	for _, item := range ips {
		switch titem := item.(type) {
		case net.IP:
			if titem.Equal(ip) {
				return true
			}

		case *net.IPNet:
			if titem.Contains(ip) {
				return true
			}
		}
	}
	return false
}

type multiBuffer struct {
	buffers [][]byte
	curBuf  int
}

func newMultiBuffer(count int, size int) *multiBuffer {
	buffers := make([][]byte, count)
	for i := 0; i < count; i++ {
		buffers[i] = make([]byte, size)
	}

	return &multiBuffer{
		buffers: buffers,
	}
}

func (mb *multiBuffer) next() []byte {
	ret := mb.buffers[mb.curBuf]
	mb.curBuf += 1
	if mb.curBuf >= len(mb.buffers) {
		mb.curBuf = 0
	}
	return ret
}

func splitPath(path string) (string, string, error) {
	pos := func() int {
		for i := len(path) - 1; i >= 0; i-- {
			if path[i] == '/' {
				return i
			}
		}
		return -1
	}()

	if pos < 0 {
		return "", "", fmt.Errorf("the path must contain a base path and a control path (%s)", path)
	}

	basePath := path[:pos]
	controlPath := path[pos+1:]

	if len(basePath) == 0 {
		return "", "", fmt.Errorf("empty base path (%s)", basePath)
	}

	if len(controlPath) == 0 {
		return "", "", fmt.Errorf("empty control path (%s)", controlPath)
	}

	return basePath, controlPath, nil
}

var rePathName = regexp.MustCompile("^[0-9a-zA-Z_\\-/]+$")

func checkPathName(name string) error {
	if !rePathName.MatchString(name) {
		return fmt.Errorf("can contain only alfanumeric characters, underscore, minus or slash")
	}

	if name[0] == '/' {
		return fmt.Errorf("can't begin with a slash")
	}

	if name[len(name)-1] == '/' {
		return fmt.Errorf("can't end with a slash")
	}

	return nil
}
