package restrictusers

import (
	"k8s.io/apiserver/pkg/admission"

	userinformer "github.com/openshift/client-go/user/informers/externalversions"
	configv1informer "github.com/openshift/client-go/config/informers/externalversions"
)

func NewInitializer(userInformer userinformer.SharedInformerFactory, configInformer configv1informer.SharedInformerFactory) admission.PluginInitializer {
    return &localInitializer{userInformer: userInformer, configInformer: configInformer}
}

type WantsUserInformer interface {
	SetUserInformer(userinformer.SharedInformerFactory)
	admission.InitializationValidator
}

type WantsConfigInformer interface {
    SetConfigInformer(configv1informer.SharedInformerFactory)
}

type localInitializer struct {
	userInformer userinformer.SharedInformerFactory
    configInformer configv1informer.SharedInformerFactory
}

// Initialize will check the initialization interfaces implemented by each plugin
// and provide the appropriate initialization data
func (i *localInitializer) Initialize(plugin admission.Interface) {
	if wants, ok := plugin.(WantsUserInformer); ok {
		wants.SetUserInformer(i.userInformer)
	}

    if wants, ok := plugin.(WantsConfigInformer); ok {
        wants.SetConfigInformer(i.configInformer)
    }
}
