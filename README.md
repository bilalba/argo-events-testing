# argo-events-testing

A testing framework for verifying the semantics of Argo Event triggers under failure.

The testing suite is kafka in, kafka out. Messages are produced to a kafka (input) topic, consumed by an EventSource and subsequently consumed by a Sensor. If a trigger condition is satisfied a message is produced to a kafka (output) topic. The suite produces `n` random messages and holds a copy of these messages in memory. When a message is consumed from the output kafka topic the message is verified against the messages in memory and the trigger semantics (at-least-once or at-most-once), if all consumed messages pass verification the test is successful, if one message fails verification (or there are unresolved messages) the test fails.

The testing suite optionally implements chaos. Sensor pods will be deleted on a specified interval. Triggers are still expected to be invoked (conforming to the specified semantics) in the presence of failure.

## Generate

Generates sample EventBus, EventSource, and Sensor objects.

```
$ python3 hack/generate.py --help
usage: generate.py [-h] [-n N] [-r R] [-b BROKERS [BROKERS ...]] [-it INPUT_TOPIC] [-ot OUTPUT_TOPIC] [-o {&&,||}] [-s {amo,alo}] [-a | --all | --no-all]

options:
  -h, --help            show this help message and exit
  -n N
  -r R
  -b BROKERS [BROKERS ...], --brokers BROKERS [BROKERS ...]
  -it INPUT_TOPIC, --input-topic INPUT_TOPIC
  -ot OUTPUT_TOPIC, --output-topic OUTPUT_TOPIC
  -o {&&,||}, --operator {&&,||}
  -s {amo,alo}, --semantics {amo,alo}
  -a, --all, --no-all
```

## Test

```
argo-events-testing test --help
Usage:
  argo-events-testing test [flags]

Flags:
  -b, --brokers stringArray    kafka brokers (default [localhost:9092])
  -c, --chaos duration         frequency to delete pod with label chaos='true' (default 0, no chaos)
  -h, --help                   help for test
  -i, --input-topic string     input kafka topic (default "input")
  -l, --local                  indicates tests running locally (not in cluster)
      --msg-timeout duration   maximum time since last recieved event (default 3m0s)
  -n, --n int                  number of events to produce (default 1)
      --name string            sensor name (default "test")
      --namespace string       namespace (default "default")
  -o, --output-topic string    input kafka topic (default "output")
  -s, --sasl                   connect to kafka with sasl
      --timeout duration       maximum time to run tests (default 1h0m0s)
  -t, --tls                    connect to kafka with tls
  -v, --verbose                kafka verbose debugging
  -w, --w duration             time to wait for late events (default 1m0s)
```
