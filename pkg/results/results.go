package results

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/Knetic/govaluate"
	"github.com/bilalba/argo-events-testing/pkg/consumer"
	"github.com/bilalba/argo-events-testing/pkg/producer"
	"github.com/bilalba/argo-events-testing/pkg/sensor/v1alpha1"
	"gonum.org/v1/gonum/stat/combin"
)

type Results struct {
	// lookup
	Triggers       map[string]*Trigger
	TriggersForDep map[string][]*Trigger
	Last           time.Time

	// result
	failures    int
	successes   int
	atMostOnce  int
	atLeastOnce int
}

func NewResults(sensor *v1alpha1.Sensor) (*Results, error) {
	triggers := map[string]*Trigger{}
	triggersForDep := map[string][]*Trigger{}

	for _, trigger := range sensor.Spec.Triggers {
		expr, err := govaluate.NewEvaluableExpression(trigger.Template.Conditions)
		if err != nil {
			return nil, err
		}

		andOps := 0
		for _, token := range expr.Tokens() {
			if token.Kind == govaluate.LOGICALOP && token.Value == "&&" {
				andOps++
			}
		}

		triggers[trigger.Template.Name] = &Trigger{
			Trigger: trigger,
			Expr:    expr,
			Terms:   andOps + 1,
		}

		for _, dep := range expr.Vars() {
			triggersForDep[dep] = append(triggersForDep[dep], triggers[trigger.Template.Name])
		}
	}

	return &Results{
		Triggers:       triggers,
		TriggersForDep: triggersForDep,
	}, nil
}

func (r *Results) Collect(ctx context.Context, produced <-chan *producer.ProducerMsg, consumed <-chan *consumer.ConsumerMsg) {
	// instantiate
	r.Last = time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-produced:
			// add event to applicable triggers
			for _, trigger := range r.TriggersForDep[msg.Dependency.Name] {
				trigger.remaining = append(trigger.remaining, msg)
			}
		case msg := <-consumed:
			r.Last = time.Now()
			r.analyze(msg)
		}
	}
}

func (r *Results) Done() bool {
	for _, trigger := range r.Triggers {
		if _, ok := trigger.Satisfied(); ok {
			return false
		}
	}

	return true
}

func (r *Results) Finalize() error {
	// check there are no remaining expected invocations
	for name, trigger := range r.Triggers {
		if params, ok := trigger.Satisfied(); ok {
			if !trigger.AtLeastOnce {
				r.atMostOnce++
				r.info(fmt.Sprintf("trigger '%s' not invoked when condition was satisfied (condition='%s' dependencies='%v', semantics='at-most-once')", name, trigger.Expr.String(), params))
			} else {
				r.failure(fmt.Sprintf("trigger '%s' not invoked when condition was satisfied (condition='%s' dependencies='%v', semantics='at-least-once')", name, trigger.Expr.String(), params))
			}
		}
	}

	if r.successes > 0 {
		fmt.Printf("✅ %d successful trigger invocations\n", r.successes)
	}
	if r.atMostOnce > 0 {
		fmt.Printf("⚠️  %d missing trigger invocations with semantics specificed as 'at-most-once'\n", r.atMostOnce)
	}
	if r.atLeastOnce > 0 {
		fmt.Printf("⚠️  %d duplicate trigger invocations with semantics specificed as 'at-least-once'\n", r.atLeastOnce)
	}
	if r.failures > 0 {
		fmt.Printf("❌ %d failures\n", r.failures)
		return fmt.Errorf("%d failures", r.failures)
	}

	return nil
}

func (r *Results) analyze(msg *consumer.ConsumerMsg) {
	params := Parameters{}
	trigger := r.Triggers[msg.Trigger]

	for _, seen := range trigger.seen {
		// duplicate invocation
		if reflect.DeepEqual(seen, msg.Value) {
			if trigger.AtLeastOnce {
				r.atLeastOnce++
				r.info(fmt.Sprintf("trigger '%s' invoked twice (semantics specified as 'at least once')", msg.Trigger))
			} else {
				r.failure(fmt.Sprintf("trigger '%s' invoked twice (semantics specified as 'at most once')", msg.Trigger))
			}
			return
		}
	}
	trigger.seen = append(trigger.seen, msg.Value)

	// construct params
	for dep, val := range msg.Value {
		if ok := trigger.containsAndShuffle(dep, val); ok {
			params[dep] = val
		} else {
			r.failure(fmt.Sprintf("trigger '%s' invoked with incorrect dependency value (condition='%s' dependencies='%v')", msg.Trigger, trigger.Expr.String(), msg.Value))
			return
		}
	}

	// check expression
	satisfied, _ := trigger.Expr.Eval(params)
	if satisfied == true {
		r.success(fmt.Sprintf("trigger '%s' invoked when condition was satisfied (condition='%s' dependencies='%v')", msg.Trigger, trigger.Expr.String(), params))
	} else {
		r.failure(fmt.Sprintf("trigger '%s' invoked when condition was not satisfied (condition='%s' dependencies='%v')", msg.Trigger, trigger.Expr.String(), params))
	}

	// number of terms should be equal to one greater than the
	// number of and operators in the expression
	// todo: verify this is true for all cases
	if len(msg.Value) != trigger.Terms {
		r.failure(fmt.Sprintf("trigger '%s' invoked with incorrect number of terms (condition='%s' dependencies='%v')", msg.Trigger, trigger.Expr.String(), params))
	}
}

func (r *Results) success(message string) {
	r.successes++
	fmt.Printf("✅ %s\n", message)
}

func (r *Results) failure(message string) {
	r.failures++
	fmt.Printf("❌ %s\n", message)
}

func (r *Results) info(message string) {
	fmt.Printf("⚠️  %s\n", message)
}

type Trigger struct {
	*v1alpha1.Trigger

	// metadata
	Expr  *govaluate.EvaluableExpression
	Terms int

	// state
	seen      []map[string]string
	extra     []*producer.ProducerMsg
	remaining []*producer.ProducerMsg
}

func (t *Trigger) Satisfied() (Parameters, bool) {
	if len(t.remaining) < t.Terms {
		return nil, false
	}

	for _, combination := range combin.Combinations(len(t.remaining), t.Terms) {
		params := Parameters{}
		for _, i := range combination {
			params[t.remaining[i].Dependency.Name] = t.remaining[i].Value
		}

		// check expression
		satisfied, _ := t.Expr.Eval(params)
		if satisfied == true {
			return params, true
		}
	}

	return nil, false
}

func (t *Trigger) containsAndShuffle(dep string, val string) bool {
	for i, msg := range t.remaining {
		if msg.Dependency.Name == dep && msg.Value == val {
			t.remaining = t.shuffle(i, dep)
			return true
		}
	}

	for i, msg := range t.extra {
		if msg.Dependency.Name == dep && msg.Value == val {
			t.extra = append(t.extra[:i], t.extra[i+1:]...)
			return true
		}
	}

	return false
}

func (t *Trigger) shuffle(index int, dep string) []*producer.ProducerMsg {
	updated := []*producer.ProducerMsg{}

	for i := 0; i < index; i++ {
		if t.remaining[i].Dependency.Name == dep {
			t.extra = append(t.extra, t.remaining[i])
		} else {
			updated = append(updated, t.remaining[i])
		}
	}

	return append(updated, t.remaining[index+1:]...)
}

type Parameters map[string]string

func (p Parameters) Get(name string) (interface{}, error) {
	if _, ok := p[name]; ok {
		return true, nil
	}
	return false, nil
}
