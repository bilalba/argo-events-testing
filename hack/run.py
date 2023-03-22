import os
import sys
import glob
import time
import subprocess

for index, filename in enumerate(glob.glob(sys.argv[1] if len(sys.argv) > 1 else 'tests/*')):
    # Setup
    print(f'Setting up test environment "{filename}"')

    p = subprocess.run([
        'kubectl',
        'apply',
        '-f',
        filename
    ])

    if p.returncode:
        sys.exit(p.returncode)

    time.sleep(60)

    # Test
    print('Running test')

    pod = f'''\
apiVersion: v1
kind: Pod
metadata:
  name: test
spec:
  serviceAccountName: test
  restartPolicy: Never
  containers:
  - name: main
    command:
    - argo-events-testing
    - test
    - --name
    - test
    - --namespace
    - default
    - -b
    - b-1.argoevents2.vywt8t.c10.kafka.us-west-2.amazonaws.com:9096
    - -b
    - b-2.argoevents2.vywt8t.c10.kafka.us-west-2.amazonaws.com:9096
    - -b
    - b-3.argoevents2.vywt8t.c10.kafka.us-west-2.amazonaws.com:9096
    - -n
    - '100'
    - -w
    - 30s
    - --chaos
    - 30s
    - --tls
    - --sasl
    - --msg-timeout
    - 10m
    - --timeout
    - 1h
    env:
    - name: SASL_USERNAME
      valueFrom:
        secretKeyRef:
          name: kafka-secret
          key: username
    - name: SASL_PASSWORD
      valueFrom:
        secretKeyRef:
          name: kafka-secret
          key: password
    image: docker.intuit.com/personal/dfarr/argo-events-testing:latest
    imagePullPolicy: Always'''

    p = subprocess.run([
        'kubectl',
        'create',
        '-f',
        '-'
    ], input=bytes(pod, 'utf-8'))

    if p.returncode:
        sys.exit(p.returncode)

    while True:
        p = subprocess.run([
            'kubectl',
            'logs',
            '-f',
            'pod/test'
        ])

        if p.returncode == 0:
            break

    # Grab results
    with open(f'results/{os.path.basename(filename)}.out', 'w') as f:
      p = subprocess.run([
          'kubectl',
          'logs',
          '--tail',
          '50',
          'pod/test'
      ], stdout=f)

    # Cleanup
    print('Cleanup')

    p = subprocess.run([
        'kubectl',
        'delete',
        'pod/test'
    ])

    if p.returncode:
        sys.exit(p.returncode)

    p = subprocess.run([
        'kubectl',
        'delete',
        '-f',
        filename
    ])

    if p.returncode:
        sys.exit(p.returncode)

    # pause in order to collect data
    print(f'Finished "{filename}"')
