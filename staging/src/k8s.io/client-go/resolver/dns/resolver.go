package dns

import (
	"context"
	"net"

	"golang.org/x/net/dns/dnsmessage"
)

const ttl = 300

// MemResolver implement an in memory resolver that receives DNS questions and
// executes the corresponding Lookup functions. If the corresponding Lookup
// function is not present, it uses the DefaultResolver ones.
type MemResolver struct {
	LookupAddr  func(ctx context.Context, addr string) (names []string, err error)
	LookupCNAME func(ctx context.Context, host string) (cname string, err error)
	LookupHost  func(ctx context.Context, host string) (addrs []string, err error)
	LookupIP    func(ctx context.Context, network, host string) ([]net.IP, error)
	LookupMX    func(ctx context.Context, name string) ([]*net.MX, error)
	LookupNS    func(ctx context.Context, name string) ([]*net.NS, error)
	LookupPort  func(ctx context.Context, network, service string) (port int, err error)
	LookupSRV   func(ctx context.Context, service, proto, name string) (cname string, addrs []*net.SRV, err error)
	LookupTXT   func(ctx context.Context, name string) ([]string, error)
	// Add new lookup functions here
	// LookupSOA https://github.com/golang/go/issues/35061
}

// ProcessDNSRequest implements dnsHandlerFunc so it can be used in a MemResolver
// transforming a DNS request to the corresponding Golang Lookup functions.
func (r *MemResolver) ProcessDNSRequest(id uint16, q dnsmessage.Question) []byte {
	buf := []byte{}
	answer := dnsmessage.NewBuilder(buf,
		dnsmessage.Header{
			ID:            id,
			Response:      true,
			Authoritative: true,
		})
	answer.EnableCompression()
	err := answer.StartQuestions()
	if err != nil {
		return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
	}
	answer.Question(q)
	err = answer.StartAnswers()
	if err != nil {
		return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
	}
	switch q.Type {
	case dnsmessage.TypeA:
		addrs, err := r.lookupIP(context.Background(), "ip4", q.Name.String())
		if err != nil {
			return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
		}

		for _, ip := range addrs {
			a := ip.To4()
			if a == nil {
				continue
			}
			err = answer.AResource(
				dnsmessage.ResourceHeader{
					Name:  q.Name,
					Class: q.Class,
					TTL:   ttl,
				},
				dnsmessage.AResource{
					A: [4]byte{a[0], a[1], a[2], a[3]},
				},
			)
			if err != nil {
				return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
			}
		}

	case dnsmessage.TypeAAAA:
		addrs, err := r.lookupIP(context.Background(), "ip6", q.Name.String())
		if err != nil {
			return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
		}

		for _, ip := range addrs {
			if ip.To16() == nil || ip.To4() != nil {
				continue
			}
			var aaaa [16]byte
			copy(aaaa[:], ip.To16())
			err = answer.AAAAResource(
				dnsmessage.ResourceHeader{
					Name:  q.Name,
					Class: q.Class,
					TTL:   ttl,
				},
				dnsmessage.AAAAResource{
					AAAA: aaaa,
				},
			)
			if err != nil {
				return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
			}
		}

	case dnsmessage.TypeNS:
		nsList, err := r.lookupNS(context.Background(), q.Name.String())
		if err != nil {
			return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
		}

		for _, ns := range nsList {
			name, err := dnsmessage.NewName(ns.Host)
			if err != nil {
				return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
			}
			err = answer.NSResource(
				dnsmessage.ResourceHeader{
					Name:  q.Name,
					Class: q.Class,
					TTL:   ttl,
				},
				dnsmessage.NSResource{
					NS: name,
				},
			)
			if err != nil {
				return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
			}
		}
	case dnsmessage.TypeCNAME:
		cname, err := r.lookupCNAME(context.Background(), q.Name.String())
		if err != nil {
			return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
		}

		name, err := dnsmessage.NewName(cname)
		if err != nil {
			return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
		}
		err = answer.CNAMEResource(
			dnsmessage.ResourceHeader{
				Name:  q.Name,
				Class: q.Class,
				TTL:   ttl,
			},
			dnsmessage.CNAMEResource{
				CNAME: name,
			},
		)
		if err != nil {
			return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
		}
	case dnsmessage.TypeSOA:
		// TODO
	case dnsmessage.TypeMX:
		mxList, err := r.lookupMX(context.Background(), q.Name.String())
		if err != nil {
			return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
		}

		for _, mx := range mxList {
			name, err := dnsmessage.NewName(mx.Host)
			if err != nil {
				return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
			}
			err = answer.MXResource(
				dnsmessage.ResourceHeader{
					Name:  q.Name,
					Class: q.Class,
					TTL:   ttl,
				},
				dnsmessage.MXResource{
					MX:   name,
					Pref: mx.Pref,
				},
			)
			if err != nil {
				return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
			}
		}
	case dnsmessage.TypeTXT:
		// You can enter a value of up to 255 characters in one string in a TXT record.
		// You can add multiple strings of 255 characters in a single TXT record.
		txt, err := r.lookupTXT(context.Background(), q.Name.String())
		if err != nil {
			return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
		}
		err = answer.TXTResource(
			dnsmessage.ResourceHeader{
				Name:  q.Name,
				Class: q.Class,
				TTL:   ttl,
			},
			dnsmessage.TXTResource{
				TXT: txt,
			},
		)
		if err != nil {
			return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
		}

	case dnsmessage.TypeSRV:
		// WIP
		_, srvList, err := r.lookupSRV(context.Background(), "", "", q.Name.String())
		if err != nil {
			return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
		}
		for _, srv := range srvList {

			target, err := dnsmessage.NewName(srv.Target)
			if err != nil {
				return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
			}
			err = answer.SRVResource(
				dnsmessage.ResourceHeader{
					Name:  q.Name,
					Class: q.Class,
					TTL:   ttl,
				},
				dnsmessage.SRVResource{
					Target:   target,
					Priority: srv.Priority,
					Weight:   srv.Weight,
					Port:     srv.Port,
				},
			)
			if err != nil {
				return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
			}
		}
	case dnsmessage.TypePTR:
		names, err := r.LookupAddr(context.Background(), q.Name.String())
		if err != nil {
			return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
		}
		for _, n := range names {
			name, err := dnsmessage.NewName(n)
			if err != nil {
				return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
			}
			err = answer.PTRResource(
				dnsmessage.ResourceHeader{
					Name:  q.Name,
					Class: q.Class,
					TTL:   ttl,
				},
				dnsmessage.PTRResource{
					PTR: name,
				},
			)
			if err != nil {
				return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
			}
		}
	default:
		return dnsErrorMessage(id, dnsmessage.RCodeNotImplemented, q)
	}

	buf, err = answer.Finish()
	if err != nil {
		return dnsErrorMessage(id, dnsmessage.RCodeServerFailure, q)
	}
	return buf
}

func (r *MemResolver) lookupAddr(ctx context.Context, addr string) (names []string, err error) {
	if r.LookupAddr != nil {
		return r.LookupAddr(ctx, addr)
	}
	return net.DefaultResolver.LookupAddr(ctx, addr)
}
func (r *MemResolver) lookupCNAME(ctx context.Context, host string) (cname string, err error) {
	if r.LookupCNAME != nil {
		return r.LookupCNAME(ctx, host)
	}
	return net.DefaultResolver.LookupCNAME(ctx, host)
}
func (r *MemResolver) lookupHost(ctx context.Context, host string) (addrs []string, err error) {
	if r.LookupHost != nil {
		return r.LookupHost(ctx, host)
	}
	return net.DefaultResolver.LookupHost(ctx, host)
}
func (r *MemResolver) lookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	if r.LookupIP != nil {
		return r.LookupIP(ctx, network, host)
	}
	return net.DefaultResolver.LookupIP(ctx, network, host)
}
func (r *MemResolver) lookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	if r.LookupMX != nil {
		return r.LookupMX(ctx, name)
	}
	return net.DefaultResolver.LookupMX(ctx, name)
}
func (r *MemResolver) lookupNS(ctx context.Context, name string) ([]*net.NS, error) {
	if r.LookupNS != nil {
		return r.LookupNS(ctx, name)
	}
	return net.DefaultResolver.LookupNS(ctx, name)
}
func (r *MemResolver) lookupPort(ctx context.Context, network, service string) (port int, err error) {
	if r.LookupPort != nil {
		return r.LookupPort(ctx, network, service)
	}
	return net.DefaultResolver.LookupPort(ctx, network, service)
}
func (r *MemResolver) lookupSRV(ctx context.Context, service, proto, name string) (cname string, addrs []*net.SRV, err error) {
	if r.LookupSRV != nil {
		return r.LookupSRV(ctx, service, proto, name)
	}
	return net.DefaultResolver.LookupSRV(ctx, service, proto, name)
}
func (r *MemResolver) lookupTXT(ctx context.Context, name string) ([]string, error) {
	if r.LookupTXT != nil {
		return r.LookupTXT(ctx, name)
	}
	return net.DefaultResolver.LookupTXT(ctx, name)
}

// MemoryResolver returns an in-memory resolver that can override golang Lookup
// functions.
func NewMemoryResolver(r *MemResolver) *net.Resolver {
	if r == nil {
		r = &MemResolver{}
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: (&MemDNSDialer{
			DNSHandler: r.ProcessDNSRequest,
		}).Dial,
	}
}
