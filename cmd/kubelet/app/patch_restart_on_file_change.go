package app

import (
	"context"
	"io/ioutil"
	"os"
	"sync"
	"time"

	"github.com/openshift/library-go/pkg/controller/fileobserver"
)

func startRestartOnFileChanges(ctx context.Context) context.Context {
	// When the kubeconfig content change, commit suicide to reload its content.
	observer, err := fileobserver.NewObserver(1 * time.Second)
	if err != nil {
		// coding error. the library needs fixing to stop returning an error
		panic(err)
	}

	// Make a context that is cancelled when the parent is closed (this happens on signals)
	// The cancel for the subcontext is called when the files change.
	wrappedContext, cancel := context.WithCancel(ctx)

	files := []string{
		"/etc/kubernetes/kubelet-ca.crt",
	}
	fileContent := map[string][]byte{}
	for _, file := range files {
		// ignore error because it means the file isn't present and we'll restart when it gets data.
		initialContent, _ := ioutil.ReadFile(file)
		fileContent[file] = initialContent
	}

	var once sync.Once
	observer.AddReactor(
		fileobserver.TerminateOnChangeReactor(func() {
			once.Do(func() {
				cancel()
				time.Sleep(60 * time.Second) // delay to allow a fairly clean shutdown if possible.  I pulled one minute from no-where.
				os.Exit(0)
			})

		}),
		fileContent,
		files...)
	observer.Run(wrappedContext.Done())

	return wrappedContext
}
