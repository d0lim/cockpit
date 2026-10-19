package reconcile

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type scriptedRunner struct {
	replies map[string]string
	fail    map[string]bool
	calls   []string
}

func (r *scriptedRunner) Run(_ context.Context, args ...string) (string, error) {
	key := strings.Join(args, " ")
	r.calls = append(r.calls, key)
	if r.fail[key] {
		return "", errors.New("daemon unreachable")
	}
	return r.replies[key], nil
}

func TestComposeLsReadsEveryProjectWithItsStatusIncludingStoppedOnes(t *testing.T) {
	r := &scriptedRunner{replies: map[string]string{
		"compose ls -a --format json": `[{"Name":"toy","Status":"running(2)","ConfigFiles":"/x/compose.yaml"},{"Name":"run-20261012-153001-997a37e","Status":"exited(12)","ConfigFiles":""}]`,
	}}
	got, err := CLI{Runner: r}.ComposeLs(context.Background())
	want := []Project{{Name: "toy", Status: "running(2)"}, {Name: "run-20261012-153001-997a37e", Status: "exited(12)"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v %v", got, err)
	}
	r.replies["compose ls -a --format json"] = "\n"
	if got, err := (CLI{Runner: r}).ComposeLs(context.Background()); err != nil || len(got) != 0 {
		t.Fatalf("empty: %+v %v", got, err)
	}
	r.replies["compose ls -a --format json"] = "NAME STATUS"
	if _, err := (CLI{Runner: r}).ComposeLs(context.Background()); err == nil {
		t.Fatal("table output accepted")
	}
}

func TestLabelledCountsContainersAndNetworksByRunIDLabelAndVolumesByComposeProject(t *testing.T) {
	p := "run-20261012-153001-997a37e"
	r := &scriptedRunner{replies: map[string]string{
		"ps -aq --filter label=io.cockpit.run-id=" + p:                "a1\nb2\nc3\n",
		"network ls -q --filter label=io.cockpit.run-id=" + p:         "n1\n",
		"volume ls -q --filter label=com.docker.compose.project=" + p: "v1\nv2\n",
	}}
	got, err := CLI{Runner: r}.Labelled(context.Background(), p)
	if err != nil || got != (Labelled{Containers: 3, Networks: 1, Volumes: 2}) {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestInfoIsFalseWhenTheDaemonDoesNotAnswer(t *testing.T) {
	r := &scriptedRunner{replies: map[string]string{"info --format {{.ServerVersion}}": "29.8.1"}, fail: map[string]bool{}}
	c := CLI{Runner: r, InfoTimeout: 1e9}
	if !c.Info(context.Background()) {
		t.Fatal("canary: answering daemon reported down")
	}
	r.fail["info --format {{.ServerVersion}}"] = true
	if c.Info(context.Background()) {
		t.Fatal("silent daemon reported up")
	}
}
