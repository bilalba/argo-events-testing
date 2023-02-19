package producer

import (
	"fmt"
	"math/rand"
	"strconv"

	"github.com/Shopify/sarama"
	"github.com/bilalba/argo-events-testing/pkg/sensor/v1alpha1"
)

type Producer struct {
	Brokers []string
	Topic   string
	Channel chan<- *ProducerMsg
}

type ProducerMsg struct {
	Dependency *v1alpha1.Dependency
	Value      string
}

func (p *Producer) Produce(config *sarama.Config, n int, dependencies []*v1alpha1.Dependency) error {
	producer, err := sarama.NewSyncProducer(p.Brokers, config)
	if err != nil {
		return nil
	}

	fmt.Printf("Producing %d messages to topic '%s'\n", n, p.Topic)

	for i := 0; i < n; i++ {
		dep := dependencies[rand.Intn(len(dependencies))]
		value := strconv.Itoa(i)

		fmt.Printf("%8d -> %s\n", i, dep.Name)

		// do this before we actually produce so that our analysis
		// can be sure the list of produced messages is correct
		p.Channel <- &ProducerMsg{
			Dependency: dep,
			Value:      value,
		}

		_, _, err := producer.SendMessage(&sarama.ProducerMessage{
			Topic: p.Topic,
			Key:   sarama.StringEncoder(dep.EventName),
			Value: sarama.StringEncoder(value),
		})

		if err != nil {
			return nil
		}
	}

	return nil
}
