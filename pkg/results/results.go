package results

import (
	"context"
	"fmt"

	"github.com/Knetic/govaluate"
	"github.com/bilalba/argo-events-testing/pkg/consumer"
	"github.com/bilalba/argo-events-testing/pkg/producer"
	"github.com/bilalba/argo-events-testing/pkg/sensor/v1alpha1"
	"gonum.org/v1/gonum/stat/combin"
)

type Results struct {
	// lookup
	Triggers       map[string]*TriggerWithExpr
	TriggersForDep map[string][]string

	// state
	Produced map[string][]*producer.ProducerMsg

	// result
	failures int
	// atMostOnce  int
	// atLeastOnce int
}

type TriggerWithExpr struct {
	*v1alpha1.Trigger
	Expr  *govaluate.EvaluableExpression
	Terms int
}

func NewResults(sensor *v1alpha1.Sensor) (*Results, error) {
	triggers := map[string]*TriggerWithExpr{}
	triggersForDep := map[string][]string{}

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

		for _, dep := range expr.Vars() {
			triggersForDep[dep] = append(triggersForDep[dep], trigger.Template.Name)
		}

		triggers[trigger.Template.Name] = &TriggerWithExpr{
			trigger,
			expr,
			andOps + 1,
		}
	}

	return &Results{
		Triggers:       triggers,
		TriggersForDep: triggersForDep,
		Produced:       map[string][]*producer.ProducerMsg{},
	}, nil
}

func (r *Results) Collect(ctx context.Context, produced <-chan *producer.ProducerMsg, consumed <-chan *consumer.ConsumerMsg) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-produced:
			for _, trigger := range r.TriggersForDep[msg.Dependency.Name] {
				r.Produced[trigger] = append(r.Produced[trigger], msg)
			}
		case msg := <-consumed:
			r.analyze(msg)
		}
	}
}

func (r *Results) Finalize() error {
	// check there are no remaining expected invocations
	for name, trigger := range r.Triggers {
		msgs := r.Produced[name]

		if len(msgs) < trigger.Terms {
			continue
		}

		// loop thru every possible permutation of messages and check
		// if resulting invocation satisfies the trigger
		for _, combination := range combin.Combinations(len(msgs), trigger.Terms) {
			params := Parameters{}
			for _, i := range combination {
				params[msgs[i].Dependency.Name] = msgs[i].Value
			}

			// check expression
			satisfied, _ := trigger.Expr.Eval(params)
			if satisfied == true {
				r.failure(fmt.Sprintf("trigger '%s' not invoked when condition was satisfied (condition='%s' dependencies='%v')", name, trigger.Expr.String(), params))
				break
			}
		}
	}

	if r.failures > 0 {
		return fmt.Errorf("%d failures", r.failures)
	}

	return nil
}

func (r *Results) analyze(msg *consumer.ConsumerMsg) {
	params := Parameters{}
	trigger := r.Triggers[msg.Trigger]

	// construct params
	for dep, val := range msg.Value {
		if ok := r.contains(msg.Trigger, dep, val); ok {
			params[dep] = val
		} else {
			r.failure(fmt.Sprintf("trigger '%s' invoked with incorrect dependency value (condition='%s' dependency='%s' value='%s')", msg.Trigger, trigger.Expr.String(), dep, val))
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

func (r *Results) contains(trigger string, dep string, val string) bool {
	var found bool
	var index int
	for i, msg := range r.Produced[trigger] {
		if msg.Dependency.Name == dep && msg.Value == val {
			found = true
			index = i
		}
	}

	// remove from list as a message can only be used by each trigger
	// once
	if found {
		r.Produced[trigger] = append(r.Produced[trigger][:index], r.Produced[trigger][index+1:]...)
	}

	return found
}

func (r *Results) success(message string) {
	fmt.Printf("✅ %s\n", message)
}

func (r *Results) failure(message string) {
	r.failures++
	fmt.Printf("❌ %s\n", message)
}

type Parameters map[string]string

func (p Parameters) Get(name string) (interface{}, error) {
	if _, ok := p[name]; ok {
		return true, nil
	}
	return false, nil
}
