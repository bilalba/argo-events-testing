import argparse
import base64
import itertools
import json
import yaml

def sasl():
    return {
        'mechanism': 'SCRAM-SHA-512',
        'userSecret': {
            'name': 'kafka-secret',
            'key': 'username'
        },
        'passwordSecret': {
            'name': 'kafka-secret',
            'key': 'password'
        }
    }

def tls():
    return {
        'insecureSkipVerify': True
    }

def generate_eventbus(brokers, suffix):
    return {
        'apiVersion': 'argoproj.io/v1alpha1',
        'kind': 'EventBus',
        'metadata': {
            'name': f'test{suffix}',
            'labels': {
                'argo-events-testing': 'true'
            }
        },
        'spec': {
            'kafka': {
                'url': ','.join(brokers),
                'sasl': sasl(),
                'tls': tls()
            }
        }
    }

def generate_eventsource(n, brokers, topic, suffix):
    return {
        'apiVersion': 'argoproj.io/v1alpha1',
        'kind': 'EventSource',
        'metadata': {
            'name': 'test',
            'labels': {
                'argo-events-testing': 'true'
            }
        },
        'spec': {
            'eventBusName': f'test{suffix}',
            'template': {
                'serviceAccountName': 'test',
                'container': {
                    'imagePullPolicy': 'Always'
                }
            },
            'kafka': {
                f'e{i}': {
                    'url': ','.join(brokers),
                    'topic': topic,
                    'partition': '0',
                    'sasl': sasl(),
                    'tls': tls(),
                    'filter': {
                        'expression': f'key=="e{i}"'
                    }
                } for i in range(n)
            }
        }
    }

def generate_sensor(n, r, brokers, topic, operator, at_least_once, suffix):
    combos = []
    for c in range(1, n+1):
        combos += itertools.combinations(range(n), c)

    return {
        'apiVersion': 'argoproj.io/v1alpha1',
        'kind': 'Sensor',
        'metadata': {
            'name': 'test',
            'labels': {
                'argo-events-testing': 'true'
            }
        },
        'spec': {
            'eventBusName': f'test{suffix}',
            'replicas': r,
            'template': {
                'metadata': {
                    'labels': {
                        'chaos': 'true'
                    }
                },
                'serviceAccountName': 'test',
                'container': {
                    'imagePullPolicy': 'Always'
                }
            },
            'dependencies': [{
                'name': f'd{i}',
                'eventSourceName': 'test',
                'eventName': f'e{i}',
            } for i in range(n)],
            'triggers': [{
                'atLeastOnce': at_least_once,
                'template': {
                    'name': f't{i}',
                    'conditions': operator.join(map(lambda i: f'd{i}', c)),
                    'kafka': {
                        'url': ','.join(brokers),
                        'topic': topic,
                        'sasl': sasl(),
                        'tls': tls(),
                        'partitioningKey': f't{i}',
                        'payload': [{
                            'src': {
                                'dependencyName': f'd{i}',
                                'dataKey': 'body'
                            },
                            'dest': f'd{i}'
                        } for i in c]
                    }
                }
            } for i, c in enumerate(combos)],
        }
    }

def generate(n, r, brokers, input_topic, output_topic, operator, semantics, suffix=''):
    # generate eventbus
    eb = generate_eventbus(brokers, suffix)

    # generate eventsource
    es = generate_eventsource(n, brokers, input_topic, suffix)

    # generate sensor
    sr = generate_sensor(n, r, brokers, output_topic, operator, semantics=='alo', suffix)

    return yaml.dump_all([eb, es, sr])

def generate_all(n, r, brokers, input_topic, output_topic, operator, semantics):
    k = 0
    for i in range(1, n+1):
        for j in range(1, min(i+1, r+1)):
            for s in semantics:
                with open(f'tests/t{i}-{j}x-{s}.yaml', 'w') as f:
                    f.write(generate(i, j, args.brokers, args.input_topic, args.output_topic, args.operator, s, suffix=f'-{k}'))
                    k = k + 1

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('-n', type=int, default=3)
    parser.add_argument('-r', type=int, default=1)
    parser.add_argument('-b', '--brokers', nargs='+', default=['localhost:9092'])
    parser.add_argument('-it', '--input-topic', default='input')
    parser.add_argument('-ot', '--output-topic', default='output')
    parser.add_argument('-o', '--operator', choices=['&&', '||'], default='&&')
    parser.add_argument('-s', '--semantics', choices=['amo', 'alo'], default='amo')
    parser.add_argument('-a', '--all', action=argparse.BooleanOptionalAction)
    args = parser.parse_args()

    if args.all:
        generate_all(args.n, args.r, args.brokers, args.input_topic, args.output_topic, args.operator, ['alo', 'amo'])
    else:
        print(generate(args.n, args.r, args.brokers, args.input_topic, args.output_topic, args.operator, args.semantics))
