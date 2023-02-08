package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/Shopify/sarama"
	"github.com/bilalba/argo-events-testing/pkg/consumer"
	"github.com/bilalba/argo-events-testing/pkg/producer"
	"github.com/bilalba/argo-events-testing/pkg/results"
	"github.com/bilalba/argo-events-testing/pkg/sensor/v1alpha1"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	brokers     []string
	inputTopic  string
	outputTopic string
	tls         bool
	verbose     bool

	name      string
	namespace string
	local     bool

	n int
	w int
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.TODO()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// read sensor from cluster
		var k8sConfig *rest.Config
		var err error

		if local {
			k8sConfig, err = clientcmd.BuildConfigFromFlags("", filepath.Join(homedir.HomeDir(), ".kube", "config"))
		} else {
			k8sConfig, err = rest.InClusterConfig()
		}

		if err != nil {
			return err
		}

		k8s, err := client.New(k8sConfig, client.Options{})
		if err != nil {
			return err
		}

		var obj unstructured.Unstructured
		obj.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "argoproj.io",
			Kind:    "Sensor",
			Version: "v1alpha1",
		})
		if err := k8s.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &obj); err != nil {
			return err
		}

		data, err := obj.MarshalJSON()
		if err != nil {
			return err
		}

		var sensor *v1alpha1.Sensor
		if err := json.Unmarshal(data, &sensor); err != nil {
			return err
		}

		// sarama config
		kafkaConfig := sarama.NewConfig()
		kafkaConfig.Producer.Return.Errors = true
		kafkaConfig.Producer.Return.Successes = true
		kafkaConfig.Producer.Idempotent = true
		kafkaConfig.Producer.RequiredAcks = sarama.WaitForAll
		kafkaConfig.Net.MaxOpenRequests = 1
		if tls {
			kafkaConfig.Net.TLS.Enable = true
		}
		if verbose {
			sarama.Logger = log.New(os.Stderr, "", log.LstdFlags)
		}

		// setup
		results, err := results.NewResults(sensor)
		if err != nil {
			return err
		}

		produced := make(chan *producer.ProducerMsg)
		consumed := make(chan *consumer.ConsumerMsg, n)

		consumer := &consumer.Consumer{
			Brokers: brokers,
			Topic:   outputTopic,
			Channel: consumed,
		}

		producer := &producer.Producer{
			Brokers: brokers,
			Topic:   inputTopic,
			Channel: produced,
		}

		// start collecting results
		go results.Collect(ctx, produced, consumed)

		// start consumer
		if err := consumer.Consume(ctx, kafkaConfig); err != nil {
			return err
		}

		// start producer
		if err := producer.Produce(kafkaConfig, n, sensor.Spec.Dependencies); err != nil {
			return err
		}

		// finished producing messages
		fmt.Printf("Consuming from topic '%s' for %ds\n", outputTopic, w)
		time.Sleep(time.Duration(w) * time.Second)

		return results.Finalize()
	},
}

func init() {
	// kafka
	testCmd.Flags().StringArrayVarP(&brokers, "brokers", "b", []string{"localhost:9092"}, "kafka brokers")
	testCmd.Flags().StringVarP(&inputTopic, "input-topic", "i", "input", "input kafka topic")
	testCmd.Flags().StringVarP(&outputTopic, "output-topic", "o", "output", "input kafka topic")
	testCmd.Flags().BoolVarP(&tls, "tls", "t", false, "connect to kafka with tls")
	testCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "kafka verbose debugging")

	// k8s
	testCmd.Flags().StringVarP(&name, "name", "", "kafka", "sensor name")
	testCmd.Flags().StringVarP(&namespace, "namespace", "", "default", "namespace")
	testCmd.Flags().BoolVarP(&local, "local", "l", false, "indicates tests running locally (not in cluster)")

	// testing
	testCmd.Flags().IntVarP(&n, "events", "n", 1, "number of events to produce")
	testCmd.Flags().IntVarP(&w, "wait", "w", 10, "number of seconds to wait once all messages have been produced")
}
