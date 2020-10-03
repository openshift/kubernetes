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
var intentCh = make(chan action, 1000)
var skipsCh = make(chan action, 1000)

var actionsByPod = map[string][]action{}
var intentsByPod = map[string][]action{}
var skipsByPod = map[string][]action{}

func init() {
	go consumeActions()
	go consumeIntent()
	go consumeSkips()
}

func Skip(ns, name string, oldPhase, newPhase v1.PodPhase) {
	skipsCh <- action{
		dumpData: false,
		when:     time.Now(),
		ns:       ns,
		name:     name,
		oldPhase: oldPhase,
		newPhase: newPhase,
	}
}

func Intent(ns, name string, oldPhase, newPhase v1.PodPhase) {
	intentCh <- action{
		dumpData: false,
		when:     time.Now(),
		ns:       ns,
		name:     name,
		oldPhase: oldPhase,
		newPhase: newPhase,
	}
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
	intentCh <- action{
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

// single threaded so I don't have to monkey with locks
func consumeIntent() {
	for {
		currAction := <-intentCh

		// we want to see data for a particular key
		if currAction.dumpData {
			actions := intentsByPod[getKey(currAction)]
			for _, currDisplay := range actions {
				fmt.Printf("   @@@@ %v\n", currDisplay)
			}
			continue
		}

		// see if we can get away without cleaning up the lists.  I think we only have a few thousand pods in e2e and these are small objects
		intentsByPod[getKey(currAction)] = append(intentsByPod[getKey(currAction)], currAction)

		if false {
			break
		}
	}
}

// single threaded so I don't have to monkey with locks
func consumeSkips() {
	for {
		currAction := <-skipsCh

		// we want to see data for a particular key
		if currAction.dumpData {
			actions := skipsByPod[getKey(currAction)]
			for _, currDisplay := range actions {
				fmt.Printf("   !!!! %v\n", currDisplay)
			}
			continue
		}

		// see if we can get away without cleaning up the lists.  I think we only have a few thousand pods in e2e and these are small objects
		skipsByPod[getKey(currAction)] = append(skipsByPod[getKey(currAction)], currAction)

		if false {
			break
		}
	}
}
