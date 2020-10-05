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
var apiPatch = make(chan action, 1000)
var intentCh = make(chan action, 1000)
var skipsCh = make(chan action, 1000)
var waitingCausedPendingCh = make(chan action, 1000)
var defaultCausedPendingCh = make(chan action, 1000)
var initCausedPendingCh = make(chan action, 1000)

var debugChannels = []debugChannelConfig{
	{ch: apiPatch, eyecatcher: "####"},
	{ch: intentCh, eyecatcher: "@@@@"},
	{ch: skipsCh, eyecatcher: "!!!!"},
	{ch: waitingCausedPendingCh, eyecatcher: "$$$$"},
	{ch: defaultCausedPendingCh, eyecatcher: "^^^^"},
	{ch: initCausedPendingCh, eyecatcher: "&&&&"},
}

type debugChannelConfig struct {
	ch         chan action
	eyecatcher string
}

func init() {
	for i := range debugChannels {
		debugChannel := debugChannels[i]
		go consume(debugChannel.ch, debugChannel.eyecatcher)
	}
}

func Dump(ns, name string) {
	for i := range debugChannels {
		debugChannel := debugChannels[i]
		dump(debugChannel.ch, ns, name)
	}
}

func dump(ch chan action, ns, name string) {
	ch <- action{
		dumpData: true,
		ns:       ns,
		name:     name,
	}
}

func queue(ch chan action, ns, name string, oldPhase, newPhase v1.PodPhase) {
	ch <- action{
		dumpData: false,
		when:     time.Now(),
		ns:       ns,
		name:     name,
		oldPhase: oldPhase,
		newPhase: newPhase,
	}
}

func InitCausedPending(ns, name string, oldPhase, newPhase v1.PodPhase) {
	queue(initCausedPendingCh, ns, name, oldPhase, newPhase)
}
func WaitCausedPending(ns, name string, oldPhase, newPhase v1.PodPhase) {
	queue(waitingCausedPendingCh, ns, name, oldPhase, newPhase)
}
func DefaultCausedPending(ns, name string, oldPhase, newPhase v1.PodPhase) {
	queue(defaultCausedPendingCh, ns, name, oldPhase, newPhase)
}
func Skip(ns, name string, oldPhase, newPhase v1.PodPhase) {
	queue(skipsCh, ns, name, oldPhase, newPhase)
}

func Intent(ns, name string, oldPhase, newPhase v1.PodPhase) {
	queue(intentCh, ns, name, oldPhase, newPhase)
}

func APIPatch(ns, name string, oldPhase, newPhase v1.PodPhase) {
	queue(apiPatch, ns, name, oldPhase, newPhase)
}

func getKey(in action) string {
	return fmt.Sprintf("%v/%v", in.ns, in.name)
}

// single threaded so I don't have to monkey with locks
func consume(ch chan action, eyecatcher string) {
	byPod := map[string][]action{}

	for {
		currAction := <-ch

		// we want to see data for a particular key
		if currAction.dumpData {
			actions := byPod[getKey(currAction)]
			for _, currDisplay := range actions {
				fmt.Printf("   %v %v\n", eyecatcher, currDisplay)
			}
			continue
		}

		// see if we can get away without cleaning up the lists.  I think we only have a few thousand pods in e2e and these are small objects
		byPod[getKey(currAction)] = append(byPod[getKey(currAction)], currAction)

		if false {
			break
		}
	}

}
