package authncache

import (
	configv1 "github.com/openshift/api/config/v1"
	configv1informer "github.com/openshift/client-go/config/informers/externalversions/config/v1"
)

type AuthnCache struct {
	authnInformer configv1informer.AuthenticationInformer
}

func NewAuthnCache(authInformer configv1informer.AuthenticationInformer) *AuthnCache {
	return &AuthnCache{
		authnInformer: authInformer,
	}
}

func (ac *AuthnCache) Authn() (*configv1.Authentication, error) {
	return ac.authnInformer.Lister().Get("cluster")
}

func (ac *AuthnCache) HasSynced() bool {
	return ac.authnInformer.Informer().HasSynced()
}
