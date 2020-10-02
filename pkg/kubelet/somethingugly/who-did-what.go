// this is ugly, no question, but it's a very easy way to track who did what to what
package somethingugly

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
)

type action struct {
	dumpData bool

	when     time.Time
	ns       string
	name     string
	oldPhase v1.PodPhase
	newPhase v1.PodPhase
}

func (a action) String() string {
	return a.ns + "/" + a.name + " " + a.when.Format(time.StampMilli) + " old=\"" + string(a.oldPhase) + "\" new=\"" + string(a.newPhase) + "\""
}

// give ourselves some slack to handle storms
var actionCh = make(chan action, 1000)

var actionsByPod = map[string][]action{}

func init() {
	go consumeActions()
}

func ChangePhase(ns, name string, oldPhase, newPhase v1.PodPhase) {
	actionCh <- action{
		dumpData: false,
		when:     time.Now(),
		ns:       ns,
		name:     name,
		oldPhase: oldPhase,
		newPhase: newPhase,
	}
}

func Dump(ns, name string) {
	actionCh <- action{
		dumpData: true,
		ns:       ns,
		name:     name,
	}
}

func getKey(in action) string {
	return fmt.Sprintf("%v/%v", in.ns, in.name)
}

// single threaded so I don't have to money with locks
func consumeActions() {
	for {
		currAction := <-actionCh

		// we want to see data for a particular key
		if currAction.dumpData {
			actions := actionsByPod[getKey(currAction)]
			for _, currDisplay := range actions {
				fmt.Printf("   #### %v\n", currDisplay)
			}
			continue
		}

		// see if we can get away without cleaning up the lists.  I think we only have a few thousand pods in e2e and these are small objects
		actionsByPod[getKey(currAction)] = append(actionsByPod[getKey(currAction)], currAction)

		if false {
			break
		}
	}
}
