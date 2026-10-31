package probe

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

const discoveryNS = "http://schemas.xmlsoap.org/ws/2005/04/discovery"
const onvifNS = "http://www.onvif.org/ver10/network/wsdl"

// ProbeONVIF reports only a correlated, namespace-valid ProbeMatch. Discovery
// does not authenticate or follow returned endpoint URLs.
func ProbeONVIF(ctx context.Context, host string) string {
	if ctx.Err() != nil {
		return ""
	}
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return ""
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	id := fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
	msg := fmt.Sprintf(`<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope" xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="%s" xmlns:dn="%s"><e:Header><w:MessageID>%s</w:MessageID><w:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To><w:Action>%s/Probe</w:Action></e:Header><e:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types></d:Probe></e:Body></e:Envelope>`, discoveryNS, onvifNS, id, discoveryNS)
	c, e := dialSocket(ctx, "udp", net.JoinHostPort(host, "3702"), 1200*time.Millisecond)
	if e != nil {
		return ""
	}
	defer c.Close()
	if _, e = c.Write([]byte(msg)); e != nil {
		return ""
	}
	buf := make([]byte, 16*1024)
	n, e := c.Read(buf)
	if e != nil || !validONVIFReply(buf[:n], id) {
		return ""
	}
	return "ONVIF ProbeMatch"
}
func validONVIFReply(data []byte, id string) bool {
	d := xml.NewDecoder(bytes.NewReader(data))
	var scopes []map[string]string
	var names []xml.Name
	related := false
	validType := false
	envelope := false
	match := false
	for {
		token, e := d.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return false
		}
		switch x := token.(type) {
		case xml.StartElement:
			ns := map[string]string{}
			if len(scopes) > 0 {
				for k, v := range scopes[len(scopes)-1] {
					ns[k] = v
				}
			}
			for _, a := range x.Attr {
				if a.Name.Space == "xmlns" {
					ns[a.Name.Local] = a.Value
				} else if a.Name.Local == "xmlns" {
					ns[""] = a.Value
				}
			}
			scopes = append(scopes, ns)
			names = append(names, x.Name)
			if len(names) == 1 {
				envelope = x.Name.Local == "Envelope" && x.Name.Space == "http://www.w3.org/2003/05/soap-envelope"
			}
			if x.Name.Local == "ProbeMatch" && x.Name.Space == discoveryNS && len(names) > 1 && names[len(names)-2].Local == "ProbeMatches" && names[len(names)-2].Space == discoveryNS {
				match = true
			}
			if x.Name.Local == "RelatesTo" && x.Name.Space == "http://schemas.xmlsoap.org/ws/2004/08/addressing" {
				var v string
				if d.DecodeElement(&v, &x) != nil {
					return false
				}
				related = strings.TrimSpace(v) == id
				scopes = scopes[:len(scopes)-1]
				names = names[:len(names)-1]
			}
			if x.Name.Local == "Types" && x.Name.Space == discoveryNS && len(names) > 1 && names[len(names)-2].Local == "ProbeMatch" {
				var v string
				if d.DecodeElement(&v, &x) != nil {
					return false
				}
				for _, q := range strings.Fields(v) {
					prefix, local, ok := strings.Cut(q, ":")
					if !ok {
						local = prefix
						prefix = ""
					}
					if ns[prefix] == onvifNS && local == "NetworkVideoTransmitter" {
						validType = true
					}
				}
				scopes = scopes[:len(scopes)-1]
				names = names[:len(names)-1]
			}
		case xml.EndElement:
			if len(scopes) > 0 {
				scopes = scopes[:len(scopes)-1]
				names = names[:len(names)-1]
			}
		}
	}
	return envelope && related && match && validType
}
