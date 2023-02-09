import argparse
import base64
import itertools
import json
import yaml

def generate_eventbus(brokers):
    return {
        'apiVersion': 'argoproj.io/v1alpha1',
        'kind': 'EventBus',
        'metadata': {
            'name': 'default',
            'labels': {
                'argo-events-testing': 'true'
            }
        },
        'spec': {
            'kafka': {
                'exotic': {
                    'url': ','.join(brokers),
                    'tls': {}
                }
            }
        }
    }

def generate_eventsource(n, brokers, topic):
    return {
        'apiVersion': 'argoproj.io/v1alpha1',
        'kind': 'EventSource',
        'metadata': {
            'name': 'kafka',
            'labels': {
                'argo-events-testing': 'true'
            }
        },
        'spec': {
            'template': {
                'container': {
                    'imagePullPolicy': 'Always'
                }
            },
            'kafka': {
                f'e{i}': {
                    'url': ','.join(brokers),
                    'topic': topic,
                    'tls': {},
                    'consumerGroup': {
                        'groupName': 'argo-events-testing'
                    },
                    'filter': {
                        'expression': f'key=="e{i}"'
                    }
                } for i in range(n)
            }
        }
    }

def generate_sensor(n, brokers, topic, replicas, operator, atLeastOnce):
    combos = []
    for c in range(1, n+1):
        combos += itertools.combinations(range(n), c)

    return {
        'apiVersion': 'argoproj.io/v1alpha1',
        'kind': 'Sensor',
        'metadata': {
            'name': 'kafka',
            'labels': {
                'argo-events-testing': 'true'
            }
        },
        'spec': {
            'replicas': replicas,
            'template': {
                'container': {
                    'imagePullPolicy': 'Always'
                }
            },
            'dependencies': [{
                'name': f'd{i}',
                'eventSourceName': 'kafka',
                'eventName': f'e{i}',
            } for i in range(n)],
            'triggers': [{
                'atLeastOnce': atLeastOnce,
                'template': {
                    'name': f't{i}',
                    'conditions': operator.join(map(lambda i: f'd{i}', c)),
                    'kafka': {
                        'url': ','.join(brokers),
                        'topic': topic,
                        'tls': {},
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

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('-n', type=int, default=3, help='number of total dependencies')
    parser.add_argument('-b', '--brokers', nargs='+', default=['localhost:9092'])
    parser.add_argument('-it', '--input-topic', default='input')
    parser.add_argument('-ot', '--output-topic', default='output')
    parser.add_argument('-r', '--replicas', type=int, default=1)
    parser.add_argument('-s', '--semantics', choices=['amo', 'alo'], default='amo')
    parser.add_argument('-o', '--operator', choices=['&&', '||'], default='&&')
    args = parser.parse_args()

    # generate eventbus
    eb = generate_eventbus(args.brokers)

    # generate eventsource
    es = generate_eventsource(args.n, args.brokers, args.input_topic)

    # generate sensor
    sr = generate_sensor(args.n, args.brokers, args.output_topic, args.replicas, args.operator, args.semantics=='alo')

    print(yaml.dump_all([eb, es, sr]))
