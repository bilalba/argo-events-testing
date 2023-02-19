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
	"github.com/bilalba/argo-events-testing/pkg/chaos"
	"github.com/bilalba/argo-events-testing/pkg/consumer"
	"github.com/bilalba/argo-events-testing/pkg/producer"
	"github.com/bilalba/argo-events-testing/pkg/results"
	"github.com/bilalba/argo-events-testing/pkg/scram"
	"github.com/bilalba/argo-events-testing/pkg/sensor/v1alpha1"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	brokers     []string
	inputTopic  string
	outputTopic string
	tls         bool
	sasl        bool
	verbose     bool

	name      string
	namespace string
	local     bool

	n          int
	w          time.Duration
	chaosFreq  time.Duration
	timeout    time.Duration
	msgTimeout time.Duration
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
		if sasl {
			kafkaConfig.Net.SASL.Enable = true
			kafkaConfig.Net.SASL.Mechanism = "SCRAM-SHA-512"
			kafkaConfig.Net.SASL.User = os.Getenv("SASL_USERNAME")
			kafkaConfig.Net.SASL.Password = os.Getenv("SASL_PASSWORD")
			kafkaConfig.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &scram.XDGSCRAMClient{HashGeneratorFcn: scram.SHA512}
			}
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
		consumed := make(chan *consumer.ConsumerMsg)

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

		// start chaos
		cctx, cancel := context.WithCancel(ctx)
		if chaosFreq != 0 {
			chaos := chaos.Chaos{
				Client:    k8s,
				Freq:      chaosFreq,
				Namespace: namespace,
			}

			go chaos.Start(cctx)
		}

		// start the clock
		t0 := time.Now()
		ticker := time.NewTicker(15 * time.Second)

		func() {
			for {
				defer ticker.Stop()

				tn := <-ticker.C
				if tn.Sub(t0) > timeout {
					fmt.Printf("Timing out after %v\n", timeout)
					return
				}
				if tn.Sub(results.Last) > msgTimeout {
					fmt.Printf("Last message recieved over %v ago, timing out\n", msgTimeout)
					return
				}
				if results.Done() {
					return
				}
			}
		}()

		cancel() // don't delete any more pods

		// stop the clock
		fmt.Printf("🕓 %s\n", time.Since(t0))

		fmt.Printf("Waiting %v for any late events...\n", w)
		time.Sleep(w)

		return results.Finalize()
	},
}

func init() {
	// kafka
	testCmd.Flags().StringArrayVarP(&brokers, "brokers", "b", []string{"localhost:9092"}, "kafka brokers")
	testCmd.Flags().StringVarP(&inputTopic, "input-topic", "i", "input", "input kafka topic")
	testCmd.Flags().StringVarP(&outputTopic, "output-topic", "o", "output", "input kafka topic")
	testCmd.Flags().BoolVarP(&tls, "tls", "t", false, "connect to kafka with tls")
	testCmd.Flags().BoolVarP(&sasl, "sasl", "s", false, "connect to kafka with sasl")
	testCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "kafka verbose debugging")

	// k8s
	testCmd.Flags().StringVarP(&name, "name", "", "kafka", "sensor name")
	testCmd.Flags().StringVarP(&namespace, "namespace", "", "default", "namespace")
	testCmd.Flags().BoolVarP(&local, "local", "l", false, "indicates tests running locally (not in cluster)")

	// testing
	testCmd.Flags().IntVarP(&n, "n", "n", 1, "number of events to produce")
	testCmd.Flags().DurationVarP(&w, "w", "w", 1*time.Minute, "time to wait for late events")
	testCmd.Flags().DurationVarP(&chaosFreq, "chaos", "c", 0, "frequency to delete pod with label chaos='true' (default 0, no chaos)")
	testCmd.Flags().DurationVarP(&timeout, "timeout", "", 1*time.Hour, "maximum time to run tests")
	testCmd.Flags().DurationVarP(&msgTimeout, "msg-timeout", "", 3*time.Minute, "maximum time since last recieved event")
}
