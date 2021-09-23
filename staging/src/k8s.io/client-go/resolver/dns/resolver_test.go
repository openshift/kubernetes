package dns

// https://github.com/golang/go/blob/master/src/net/lookup_test.go

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func hasSuffixFold(s, suffix string) bool {
	return strings.HasSuffix(strings.ToLower(s), strings.ToLower(suffix))
}

func TestLookupNS(t *testing.T) {
	t.Parallel()

	var lookupGmailNSTests = []struct {
		name, host string
	}{
		{"gmail.com", "google.com."},
		{"gmail.com.", "google.com."},
		{"gmail1.com.", "google2.com."},
	}

	f := &MemResolver{
		LookupNS: func(ctx context.Context, name string) ([]*net.NS, error) {
			switch name {
			case "gmail.com.":
				return []*net.NS{&net.NS{Host: "google.com."}}, nil
			case "gmail1.com.":
				return []*net.NS{&net.NS{Host: "google2.com."}, &net.NS{Host: "Google2.com."}}, nil
			default:
				return nil, fmt.Errorf("error")
			}
		},
	}
	r := NewMemoryResolver(f)

	for i := 0; i < len(lookupGmailNSTests); i++ {
		tt := lookupGmailNSTests[i]
		nss, err := r.LookupNS(context.Background(), tt.name)
		if err != nil {
			t.Fatal(err)
		}
		if len(nss) == 0 {
			t.Error("got no record")
		}
		for _, ns := range nss {
			if !hasSuffixFold(ns.Host, tt.host) {
				t.Errorf("got %v; want a record containing %s", ns, tt.host)
			}
		}
	}
}

func TestLookupTXT(t *testing.T) {
	t.Parallel()

	var lookupGmailTXTTests = []struct {
		name, txt, host string
	}{
		{"gmail.com", "spf", "fakegoogle.com"},
		{"gmail.com.", "spf", "fakegoogle.com"},
		{"gmail1.com.", "spf", "fakegoogle2.com"},
	}

	f := &MemResolver{
		LookupTXT: func(ctx context.Context, name string) ([]string, error) {
			switch name {
			case "gmail.com.":
				return []string{"spf", "google.com", "fakegoogle.com"}, nil
			case "gmail1.com.":
				return []string{"spf", "google.com", "fakegoogle2.com"}, nil
			default:
				return nil, fmt.Errorf("error")
			}
		},
	}
	r := NewMemoryResolver(f)
	for i := 0; i < len(lookupGmailTXTTests); i++ {
		tt := lookupGmailTXTTests[i]
		txts, err := r.LookupTXT(context.Background(), tt.name)
		if err != nil {
			t.Fatal(err)
		}
		if len(txts) == 0 {
			t.Error("got no record")
		}
		found := false
		for _, txt := range txts {
			if strings.Contains(txt, tt.txt) && (strings.HasSuffix(txt, tt.host) || strings.HasSuffix(txt, tt.host+".")) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("got %v; want a record containing %s, %s", txts, tt.txt, tt.host)
		}
	}
}

func TestLookupAddr(t *testing.T) {
	t.Parallel()

	var lookupGooglePublicDNSAddrTests = []string{
		"8.8.8.8",
		"8.8.4.4",
		"2001:4860:4860::8888",
		"2001:4860:4860::8844",
	}

	f := &MemResolver{
		LookupAddr: func(ctx context.Context, addr string) (names []string, err error) {
			switch addr {
			default:
				return []string{"test.google.com.", "test.golang.com."}, nil
			}
		},
	}
	r := NewMemoryResolver(f)

	for _, ip := range lookupGooglePublicDNSAddrTests {
		names, err := r.LookupAddr(context.Background(), ip)
		if err != nil {
			t.Fatal(err)
		}
		if len(names) != 2 {
			t.Errorf("expected 2 records, got %d records", len(names))
		}
		if names[0] != "test.google.com." {
			t.Errorf("got %q; want a record test.google.com.", names[0])
		}
		if names[1] != "test.golang.com." {
			t.Errorf("got %q; want a record test.golang.com.", names[1])
		}
	}

}

func TestLookupLongTXT(t *testing.T) {
	t.Skip()
	// resolver_test.go:164: lookup golang.rsc.io on 127.0.0.53:53: cannot unmarshal DNS message
	want := []string{
		strings.Repeat("abcde12345", 25), // 10 * 25  = 250
		strings.Repeat("abcde12345", 25), // 10 * 25  = 250
		strings.Repeat("abcde12345", 25), // 10 * 25  = 250
		strings.Repeat("abcde12345", 25), // 10 * 25  = 250
		"gophers rule",
	}
	f := &MemResolver{
		LookupTXT: func(ctx context.Context, name string) ([]string, error) {
			switch name {
			default:
				return want, nil
			}
		},
	}
	r := NewMemoryResolver(f)
	txts, err := r.LookupTXT(context.Background(), "golang.rsc.io")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(txts)

	if !reflect.DeepEqual(txts, want) {
		t.Fatalf("LookupTXT golang.rsc.io incorrect\nhave %q\nwant %q", txts, want)
	}
}

func TestLookupIP(t *testing.T) {
	t.Parallel()

	var lookupGoogleIPTests = []struct {
		name    string
		network string
	}{
		{"google.com", "ip4"},
		{"google.com.", "ip"},
		{"fakegoogle.com.", "ip6"},
	}

	f := &MemResolver{
		LookupIP: func(ctx context.Context, network, host string) ([]net.IP, error) {
			if network == "ip6" {
				return []net.IP{net.ParseIP("2001:db8::1")}, nil
			}

			switch host {
			default:
				return []net.IP{net.ParseIP("127.8.8.8"), net.ParseIP("169.254.0.1")}, nil
			}

		},
	}
	r := NewMemoryResolver(f)
	for _, tt := range lookupGoogleIPTests {
		ips, err := r.LookupIP(context.Background(), tt.network, tt.name)
		if err != nil {
			t.Fatal(err)
		}
		if len(ips) == 0 {
			t.Error("got no record")
		}
		if tt.network == "ip6" {
			if ips[0].String() != "2001:db8::1" {
				t.Errorf("got %v; want IP 2001:db8::1", ips[0])
			}
			return
		}

		if ips[0].String() != "127.8.8.8" {
			t.Errorf("got %v; want IP 127.8.8.8", ips[0])
		}

		if ips[1].String() != "169.254.0.1" {
			t.Errorf("got %v; want IP 169.254.0.1", ips[1])
		}
	}
}

func TestLookupCNAME(t *testing.T) {
	t.Parallel()
	// This is actually doing A and AAAA requests
	t.Skip()
	var lookupCNAMETests = []struct {
		name, cname string
	}{
		{"www.iana.org", "icann.org."},
		{"www.iana.org.", "icann.org."},
		{"www.google.com", "google.com."},
	}

	f := &MemResolver{
		LookupCNAME: func(ctx context.Context, host string) (string, error) {
			switch {
			case strings.Contains(host, "iana"):
				return "icann.org.", nil
			default:
				return "google.com.", nil
			}
		},
		LookupIP: func(ctx context.Context, network, host string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("127.8.8.8"), net.ParseIP("169.254.0.1")}, nil
		},
	}
	r := NewMemoryResolver(f)

	for i := 0; i < len(lookupCNAMETests); i++ {
		tt := lookupCNAMETests[i]
		cname, err := r.LookupCNAME(context.Background(), tt.name)
		if err != nil {
			t.Fatal(err)
		}
		if !hasSuffixFold(cname, tt.cname) {
			t.Errorf("got %s; want a record containing %s", cname, tt.cname)
		}
	}
}

func TestLookupMX(t *testing.T) {
	t.Parallel()
	var lookupMXTests = []struct {
		name, host string
	}{
		{"gmail.com", "google.com."},
		{"gmail2.com.", "google2.com."},
	}

	f := &MemResolver{
		LookupMX: func(ctx context.Context, host string) ([]*net.MX, error) {
			var mxHost string
			switch host {
			case "gmail.com.":
				mxHost = "google.com."
			case "gmail2.com.":
				mxHost = "google2.com."
			default:
				return nil, fmt.Errorf("error")
			}
			mxs := []*net.MX{
				&net.MX{
					Host: mxHost,
					Pref: 10,
				},
				&net.MX{
					Host: mxHost,
					Pref: 10,
				},
			}
			return mxs, nil
		},
	}
	r := NewMemoryResolver(f)

	for i := 0; i < len(lookupMXTests); i++ {
		tt := lookupMXTests[i]
		mxs, err := r.LookupMX(context.Background(), tt.name)
		if err != nil {
			t.Fatal(err)
		}
		if len(mxs) == 0 {
			t.Error("got no record")
		}
		for _, mx := range mxs {
			if !hasSuffixFold(mx.Host, tt.host) {
				t.Errorf("got %s; want a record containing %s", mx.Host, tt.name)
			}
			if mx.Pref != 10 {
				t.Errorf("got %d; want a record prefix of 10", mx.Pref)
			}
		}
	}
}
