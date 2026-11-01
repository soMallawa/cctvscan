package secrets

import (
	"bytes"
	"strings"
	"testing"
)

func TestBox(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	b, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("credential-do-not-log")
	a, err := b.Seal("proxy/v1", plain)
	if err != nil {
		t.Fatal(err)
	}
	c, err := b.Seal("proxy/v1", plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, c) {
		t.Fatal("nonce reused")
	}
	got, err := b.Open("proxy/v1", a)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatal("roundtrip failed")
	}
	other, _ := New(bytes.Repeat([]byte{8}, 32))
	for _, tc := range []struct {
		box  *Box
		aad  string
		data []byte
	}{{b, "wrong", a}, {other, "proxy/v1", a}, {b, "proxy/v1", nil}, {b, "proxy/v1", []byte{1}}, {b, "proxy/v1", append([]byte{}, a[:len(a)-1]...)}} {
		_, err := tc.box.Open(tc.aad, tc.data)
		if err == nil {
			t.Fatal("accepted unauthenticated ciphertext")
		}
		if strings.Contains(err.Error(), string(plain)) {
			t.Fatal("secret leaked")
		}
	}
	for _, n := range []int{0, 16, 31, 33} {
		if _, err := New(make([]byte, n)); err == nil {
			t.Fatal("accepted invalid key")
		}
	}
}
