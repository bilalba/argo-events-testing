package consumer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/Shopify/sarama"
	"github.com/google/uuid"
)

type Consumer struct {
	Brokers []string
	Topic   string
	Channel chan<- *ConsumerMsg
	ready   chan interface{}
}

type ConsumerMsg struct {
	Trigger string
	Value   map[string]string
}

func (c *Consumer) Consume(ctx context.Context, config *sarama.Config) error {
	consumer, err := sarama.NewConsumerGroup(c.Brokers, uuid.New().String(), config)
	if err != nil {
		return err
	}

	c.ready = make(chan interface{})

	go func() {
		if err := consumer.Consume(ctx, []string{c.Topic}, c); err != nil {
			fmt.Println(err)
			return
		}

		if err := ctx.Err(); err != nil {
			fmt.Println(err)
			return
		}
	}()

	<-c.ready
	return nil
}

func (c *Consumer) Setup(session sarama.ConsumerGroupSession) error {
	close(c.ready)
	return nil
}

func (c *Consumer) Cleanup(session sarama.ConsumerGroupSession) error {
	return nil
}

func (c *Consumer) Close() error {
	return nil
}

func (c *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg := <-claim.Messages():
			var values map[string]string
			if err := json.Unmarshal(msg.Value, &values); err != nil {
				return err
			}

			for key, val := range values {
				value, err := base64.StdEncoding.DecodeString(val)
				if err != nil {
					return err
				}

				values[key] = string(value)
			}

			c.Channel <- &ConsumerMsg{
				Trigger: string(msg.Key),
				Value:   values,
			}

			session.MarkMessage(msg, "")
		case <-session.Context().Done():
			return nil
		}
	}
}
