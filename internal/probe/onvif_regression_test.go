package probe

import "testing"

func TestONVIFRequiresCorrelatedProbeMatch(t *testing.T) {
	id := "urn:uuid:fixture"
	good := []byte(`<e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope" xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dn="http://www.onvif.org/ver10/network/wsdl"><e:Header><w:RelatesTo>urn:uuid:fixture</w:RelatesTo></e:Header><e:Body><d:ProbeMatches><d:ProbeMatch><d:Types>dn:NetworkVideoTransmitter</d:Types></d:ProbeMatch></d:ProbeMatches></e:Body></e:Envelope>`)
	if !validONVIFReply(good, id) {
		t.Fatal("valid correlated response rejected")
	}
	if validONVIFReply(good, "other") || validONVIFReply([]byte("random UDP bytes"), id) {
		t.Fatal("uncorrelated/arbitrary reply accepted")
	}
}
