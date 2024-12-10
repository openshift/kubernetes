package restrictusers

import configv1 "github.com/openshift/api/config/v1"

type fakeAuthnCache struct {
	authn     *configv1.Authentication
	err       error
	hasSynced bool
}

func (f *fakeAuthnCache) Authn() (*configv1.Authentication, error) {
	return f.authn, f.err
}

func (f *fakeAuthnCache) HasSynced() bool {
	return f.hasSynced
}
