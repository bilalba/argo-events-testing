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

    time.sleep(15)

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
    - '500'
    - -w
    - 30s
    - --chaos
    - 30s
    - --tls
    - --sasl
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
    imagePullPolicy: Always
    resources:
      requests:
        cpu: 1
        memory: 12G
      limits:
        cpu: 1
        memory: 12G'''

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
            f'pod/test'
        ], input=bytes(pod, 'utf-8'))

        if p.returncode == 0:
            break

    # Cleanup
    print('Cleanup')

    p = subprocess.run([
        'kubectl',
        'delete',
        f'pod/test'
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
    time.sleep(15)
