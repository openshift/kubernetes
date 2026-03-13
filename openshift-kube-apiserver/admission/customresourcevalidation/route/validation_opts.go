package route

import (
	routecommon "github.com/openshift/library-go/pkg/route"
)

type RouteValidationOptionGetter interface {
	GetValidationOptions() routecommon.RouteValidationOptions
}

type RouteValidationOpts struct {
	opts routecommon.RouteValidationOptions
}

var _ RouteValidationOptionGetter = &RouteValidationOpts{}

func NewRouteValidationOpts() *RouteValidationOpts {
	return &RouteValidationOpts{
		opts: routecommon.RouteValidationOptions{
			AllowExternalCertificates: true,
		},
	}
}

func (o *RouteValidationOpts) GetValidationOptions() routecommon.RouteValidationOptions {
	return o.opts
}
